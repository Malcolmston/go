defmodule ObanParity.Runner do
  @moduledoc """
  Upstream (Elixir Oban) side of the cross-language parity harness.

  Speaks JSON Lines on stdio: one request object per line in, one response
  object per line out, in the same order. Never exits on a failing case — any
  raise/throw/exit is reported as `ok: false`.

  Only database-free parts of Oban are reachable here: `Oban.Cron.Expression`,
  `Oban.Backoff`, `Oban.Worker.backoff/1` and `Oban.Job.new/2` changeset
  building. Everything that touches a queue needs PostgreSQL and is out of
  scope; see ../COVERAGE.md.
  """

  alias Oban.Cron.Expression, as: Cron
  alias Oban.Job

  # Oban.Worker's @clamped_max: attempts are scaled into 1..20 when
  # max_attempts exceeds it. Mirrored here because Oban.Worker.backoff/1 folds
  # jitter into the same call and the deterministic component cannot otherwise
  # be observed.
  @clamped_max 20

  def main(_argv), do: loop()

  defp loop do
    case IO.read(:stdio, :line) do
      :eof ->
        :ok

      {:error, _reason} ->
        :ok

      line ->
        line = String.trim(line)

        if line != "" do
          IO.write([Jason.encode!(respond(line)), "\n"])
        end

        loop()
    end
  end

  defp respond(line) do
    case Jason.decode(line) do
      {:ok, req} when is_map(req) ->
        run(Map.get(req, "id"), Map.get(req, "fn"), Map.get(req, "args") || [])

      other ->
        %{id: nil, ok: false, error: "bad request line: #{inspect(other)}"}
    end
  end

  defp run(id, fname, args) do
    %{id: id, ok: true, value: dispatch(fname, args)}
  rescue
    error -> %{id: id, ok: false, error: Exception.message(error)}
  catch
    kind, reason -> %{id: id, ok: false, error: "#{kind}: #{inspect(reason)}"}
  end

  # ---------------------------------------------------------------- cron ----

  # Oban.Cron.Expression.parse!/1
  defp dispatch("cron_parse", [spec]) do
    _cron = Cron.parse!(spec)
    %{valid: true}
  end

  # Oban.Cron.Expression.now?/2
  defp dispatch("cron_now", [spec, ref]) do
    Cron.now?(Cron.parse!(spec), dt(ref))
  end

  # Oban.Cron.Expression.next_at/2
  defp dispatch("cron_next", [spec, ref]) do
    case Cron.next_at(Cron.parse!(spec), dt(ref)) do
      :unknown -> "unknown"
      time -> iso(time)
    end
  end

  # Oban.Cron.Expression.last_at/2
  defp dispatch("cron_prev", [spec, ref]) do
    case Cron.last_at(Cron.parse!(spec), dt(ref)) do
      :unknown -> "unknown"
      time -> iso(time)
    end
  end

  # Composition of next_at/2, matching the port's Schedule.Upcoming.
  defp dispatch("cron_upcoming", [spec, ref, n]) when is_integer(n) do
    cron = Cron.parse!(spec)

    {_last, acc} =
      Enum.reduce_while(List.duplicate(nil, max(n, 0)), {dt(ref), []}, fn _, {cur, acc} ->
        case Cron.next_at(cron, cur) do
          :unknown -> {:halt, {cur, acc}}
          time -> {:cont, {time, [iso(time) | acc]}}
        end
      end)

    Enum.reverse(acc)
  end

  # ------------------------------------------------------------- backoff ----

  # Oban.Backoff.exponential/2
  defp dispatch("backoff_exponential", [attempt, mult, max_pow, min_pad]) do
    Oban.Backoff.exponential(attempt, mult: mult, max_pow: max_pow, min_pad: min_pad)
  end

  # Deterministic component of Oban.Worker.backoff/1, in seconds.
  defp dispatch("worker_backoff_base", [attempt, max_attempts]) do
    worker_base(attempt, max_attempts)
  end

  # Which jitter the default worker backoff applies: :inc, :dec, :both, none.
  defp dispatch("worker_backoff_jitter", []), do: "inc"

  # Does the real (jittered) Oban.Worker.backoff/1 stay inside its documented
  # bounds of [base, base + 10%]?
  defp dispatch("worker_backoff_in_bounds", [attempt, max_attempts]) do
    base = worker_base(attempt, max_attempts)
    actual = Oban.Worker.backoff(%Job{attempt: attempt, max_attempts: max_attempts})

    actual >= base and actual <= base + trunc(base * 0.1)
  end

  # ----------------------------------------------------------------- job ----

  # Oban.Job.new/2 normalisation, without inserting anything.
  defp dispatch("job_new", [worker, args, opts]) do
    job =
      args
      |> Job.new(job_opts(worker, opts))
      |> Ecto.Changeset.apply_action!(:insert)

    %{
      queue: job.queue,
      worker: job.worker,
      max_attempts: job.max_attempts,
      # The oban_jobs.priority column defaults to 0; the changeset leaves it nil.
      priority: job.priority || 0,
      state: job.state,
      args: canon(job.args)
    }
  end

  defp dispatch(fname, args) do
    throw({:unsupported_fn, fname, length(args)})
  end

  # ----------------------------------------------------------- helpers ------

  defp worker_base(attempt, max_attempts) do
    clamped =
      if max_attempts <= @clamped_max do
        attempt
      else
        round(attempt / max_attempts * @clamped_max)
      end

    Oban.Backoff.exponential(clamped, mult: 1, max_pow: 100, min_pad: 15)
  end

  @job_opt_keys %{
    "queue" => :queue,
    "max_attempts" => :max_attempts,
    "priority" => :priority,
    "schedule_in" => :schedule_in,
    "state" => :state
  }

  defp job_opts(worker, opts) do
    opts = opts || %{}

    Enum.reduce(opts, [worker: worker], fn {key, value}, acc ->
      case Map.fetch(@job_opt_keys, key) do
        {:ok, atom} -> [{atom, value} | acc]
        :error -> throw({:unknown_job_opt, key})
      end
    end)
  end

  defp canon(map) when is_map(map) do
    map
    |> Enum.map(fn {k, v} -> [to_string(k), canon(v)] end)
    |> Enum.sort_by(&hd/1)
  end

  defp canon(list) when is_list(list), do: Enum.map(list, &canon/1)
  defp canon(value), do: value

  defp dt(ref) do
    case DateTime.from_iso8601(ref) do
      {:ok, time, _offset} -> time
      {:error, reason} -> throw({:bad_datetime, ref, reason})
    end
  end

  defp iso(time) do
    time
    |> DateTime.truncate(:second)
    |> DateTime.to_iso8601()
  end
end
