defmodule LiveViewParity.Templates do
  @moduledoc """
  The upstream (HEEx) side of every logical template used by the harness.

  Each function here has a same-named counterpart in the Go runner built from
  the Go port's own `{{ ... }}` template syntax. Template *syntax* is explicitly
  out of scope for the comparison (see ../COVERAGE.md); only the diff structure
  the two engines emit for the same logical markup is compared, so the HEEx here
  is written on one line with no leading or trailing whitespace to keep HEEx's
  whitespace trimming from changing the statics.
  """
  use Phoenix.Component

  # Two slots, exactly the port's built-in Counter template.
  def counter(assigns),
    do: ~H|<div class="counter"><h1>{@label}</h1><span class="value">{@count}</span></div>|

  # Two adjacent slots: the static between them is the empty string.
  def pair(assigns), do: ~H|<p>{@a}{@b}</p>|

  # A slot in leading position: statics[0] is the empty string.
  def lead(assigns), do: ~H|{@a}<p>x</p>|

  # A slot in trailing position: the last static is the empty string.
  def trail(assigns), do: ~H|<p>x</p>{@a}|

  # No slots at all.
  def plain(assigns), do: ~H|<p>hi</p>|

  # One slot.
  def one(assigns), do: ~H|<i>{@a}</i>|

  # A nested sub-template (function component), i.e. a nested Rendered.
  def nest(assigns), do: ~H|<div><b>{@a}</b><section><.inner v={@v} /></section></div>|
  defp inner(assigns), do: ~H|<i>{@v}</i>|

  # Escaped interpolation of an untrusted value.
  def noraw(assigns), do: ~H|<div>{@html}</div>|

  # Raw / safe passthrough.
  def rawhtml(assigns), do: ~H|<div>{Phoenix.HTML.raw(@html)}</div>|

  # A slot naming an assign that is not provided.
  def missing(assigns), do: ~H|<p>{@nope}</p>|

  # A list comprehension.
  def items(assigns), do: ~H|<ul><li :for={i <- @items}>{i}</li></ul>|

  # A comprehension with two slots per entry.
  def rows(assigns), do: ~H|<table><tr :for={r <- @items}><td>{r}</td><td>{r}</td></tr></table>|

  # The parent of one stateful component.
  def withcomp(assigns),
    do: ~H|<div><h1>{@title}</h1>{live_component(%{module: LiveViewParity.Badge, id: "b1", text: @text})}</div>|

  # The parent of two stateful components of the same module.
  def twocomp(assigns),
    do:
      ~H|<div><h1>{@title}</h1>{live_component(%{module: LiveViewParity.Badge, id: "b1", text: @text})}{live_component(%{module: LiveViewParity.Badge, id: "b2", text: @text})}</div>|
end

defmodule LiveViewParity.Badge do
  @moduledoc """
  Upstream mirror of the Go runner's stateful component: its own `hits` counter
  plus a `text` handed down by the parent.
  """
  use Phoenix.LiveComponent

  def mount(socket), do: {:ok, assign(socket, :hits, 0)}

  def update(assigns, socket), do: {:ok, assign(socket, assigns)}

  def handle_event("bump", _payload, socket) do
    hits = Map.fetch!(socket.assigns, :hits)
    {:noreply, assign(socket, :hits, hits + 1)}
  end

  def handle_event("set", payload, socket) do
    {:noreply, assign(socket, :hits, Map.get(payload, "value"))}
  end

  def handle_event(_other, _payload, socket), do: {:noreply, socket}

  def render(assigns), do: ~H|<span class="badge">{@text}:{@hits}</span>|
end

defmodule LiveViewParity.Runner do
  @moduledoc """
  Upstream (Phoenix LiveView) side of the cross-language parity harness.

  Speaks JSON Lines on stdio: one request object per line in, one response
  object per line out, in the same order. Never exits on a failing case — any
  raise/throw/exit becomes `ok: false`.

  No Phoenix endpoint, router, PubSub or WebSocket is started. The only upstream
  machinery exercised is the *pure* rendering pipeline:

    * `Phoenix.LiveView.Engine` / `Phoenix.Component.sigil_H/2` — compiling HEEx
      into a `%Phoenix.LiveView.Rendered{}` static/dynamic split.
    * `Phoenix.LiveView.Diff.render/4` — turning a `Rendered` into the wire diff,
      threading fingerprints and component state so the *second* render yields a
      minimal diff.
    * `Phoenix.LiveView.Diff.write_component/4` — routing an event into a
      stateful component and producing its `"c"` diff.

  That is deliberately the whole comparable surface; see ../COVERAGE.md.
  """

  alias Phoenix.LiveView.Diff
  alias LiveViewParity.Templates

  @socket %Phoenix.LiveView.Socket{}

  # Assign names the harness is allowed to use. JSON gives us string keys and
  # HEEx needs atoms; converting through a fixed whitelist keeps the runner from
  # creating atoms from input.
  @assign_keys ~w(label count a b v html nope items title text hits)

  def main(_argv), do: loop()

  defp loop do
    case IO.read(:stdio, :line) do
      :eof ->
        :ok

      {:error, _reason} ->
        :ok

      line ->
        line = String.trim(line)
        if line != "", do: IO.write([Jason.encode!(respond(line)), "\n"])
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

  # ------------------------------------------------------- template renders ----

  # The full/initial diff for a single render: statics under "s" plus every
  # dynamic slot.
  defp dispatch("render_full", [tmpl, assigns]) do
    {diff, _prints, _components} = render_once(tmpl, assigns, nil)
    canon(diff)
  end

  # The interleaved HTML, so escaping can be compared as a string too.
  defp dispatch("render_html", [tmpl, assigns]) do
    {diff, _prints, _components} = render_once(tmpl, assigns, nil)
    diff |> Diff.to_iodata() |> IO.iodata_to_binary()
  end

  # Render twice and return the SECOND diff. `__changed__` for the second render
  # marks exactly the assigns whose value actually differs, which is what a
  # LiveView that only `assign/3`s real changes produces.
  defp dispatch("render_diff", [tmpl, before, later]) do
    render_twice(tmpl, before, later, changed_keys(before, later))
  end

  # Same, but every assign is marked changed — the shape produced by a LiveView
  # that re-assigns each key unconditionally, even to the same value.
  defp dispatch("render_diff_reassign", [tmpl, before, later]) do
    render_twice(tmpl, before, later, Map.keys(later))
  end

  # ------------------------------------------------------------- "sessions" ----

  # Mount and return the initial (full) diff.
  defp dispatch("session_initial", [view, params]) do
    {diff, _state} = session_mount(view, params)
    canon(diff)
  end

  # The same initial diff with the component static cross-references left INTACT,
  # i.e. exactly what goes on the wire. Upstream ships the statics of only the
  # first component of a module and points every later one at it with an integer
  # `"s"`; the normalised `session_initial` hides that, so this variant exists to
  # compare it head-on.
  defp dispatch("session_initial_wire", [view, params]) do
    {diff, _state} = session_mount(view, params)
    canon(diff, false)
  end

  # Mount, ship the initial diff, then dispatch one event and return its diff.
  defp dispatch("session_event", [view, params, event, payload]) do
    {_initial, state} = session_mount(view, params)
    {diff, _state} = session_event(state, event, payload)
    canon(diff)
  end

  # Identical upstream: LiveView has no transport on which the client can receive
  # an event diff without first having received the document. See ../COVERAGE.md.
  defp dispatch("session_event_no_initial", [view, params, event, payload]) do
    dispatch("session_event", [view, params, event, payload])
  end

  # Route an event to the stateful component addressed by cid.
  defp dispatch("component_event", [view, params, cid, event, payload]) when is_integer(cid) do
    {_initial, state} = session_mount(view, params)

    case Diff.write_component(@socket, cid, state.components, fn csocket, component ->
           {:noreply, updated} = component.handle_event(event, payload, csocket)
           {updated, :noop}
         end) do
      {diff, components, :noop} ->
        # Re-render the parent exactly as the runtime does after a component
        # event, so the answer includes anything the parent contributes.
        {parent, _prints, _components} =
          Diff.render(@socket, render_template(state.view, put_changed(state.assigns, [])),
                      state.prints, components)

        canon(deep_merge(diff, parent))

      :error ->
        throw({:no_such_cid, cid})
    end
  end

  defp dispatch(fname, args), do: throw({:unsupported_fn, fname, length(args)})

  # ------------------------------------------------------------------ core ----

  defp render_once(tmpl, assigns, changed) do
    rendered = render_template(tmpl, put_changed(atomise(assigns), changed))
    Diff.render(@socket, rendered, Diff.new_fingerprints(), Diff.new_components())
  end

  defp render_twice(tmpl, before, later, changed) do
    {_d1, prints, components} = render_once(tmpl, before, nil)

    rendered =
      render_template(tmpl, put_changed(atomise(later), Enum.map(changed, &to_atom/1)))

    {diff, _prints, _components} = Diff.render(@socket, rendered, prints, components)
    canon(diff)
  end

  # `__changed__: nil` means "everything is new", which is what the first render
  # of a mounted view does. A list of keys means only those were assigned.
  defp put_changed(assigns, nil), do: Map.put(assigns, :__changed__, nil)

  defp put_changed(assigns, keys) when is_list(keys),
    do: Map.put(assigns, :__changed__, Map.new(keys, &{to_atom(&1), true}))

  defp changed_keys(before, later) do
    before = atomise(before)
    later = atomise(later)

    later
    |> Map.keys()
    |> Enum.filter(fn k -> Map.get(before, k, :__absent__) != Map.get(later, k) end)
  end

  defp render_template(name, assigns) do
    case name do
      "counter" -> Templates.counter(assigns)
      "pair" -> Templates.pair(assigns)
      "lead" -> Templates.lead(assigns)
      "trail" -> Templates.trail(assigns)
      "plain" -> Templates.plain(assigns)
      "one" -> Templates.one(assigns)
      "nest" -> Templates.nest(assigns)
      "noraw" -> Templates.noraw(assigns)
      "raw" -> Templates.rawhtml(assigns)
      "missing" -> Templates.missing(assigns)
      "items" -> Templates.items(assigns)
      "rows" -> Templates.rows(assigns)
      "withcomp" -> Templates.withcomp(assigns)
      "twocomp" -> Templates.twocomp(assigns)
      other -> throw({:unknown_template, other})
    end
  end

  # ------------------------------------------------------------- "sessions" ----

  # A minimal stand-in for the Go port's Session: mount produces assigns and the
  # first (full) diff; each event updates assigns, re-renders with the changed
  # set the handler produced, and diffs against the previous fingerprints.
  defp session_mount(view, params) do
    tmpl = view_template(view)
    assigns = mount_assigns(view, params || %{})
    rendered = render_template(tmpl, put_changed(assigns, nil))
    {diff, prints, components} = Diff.render(@socket, rendered, Diff.new_fingerprints(), Diff.new_components())
    {diff, %{view: tmpl, assigns: assigns, prints: prints, components: components}}
  end

  defp session_event(state, event, payload) do
    {assigns, hinted} = handle_event(state.assigns, event, payload || %{})

    # Phoenix.Component.assign/3 does NOT mark a key changed when it is assigned
    # the value it already holds, so the mirror must not either.
    changed = Enum.filter(hinted, fn k -> Map.get(state.assigns, k) !== Map.get(assigns, k) end)
    rendered = render_template(state.view, put_changed(assigns, changed))
    {diff, prints, components} = Diff.render(@socket, rendered, state.prints, state.components)
    {diff, %{state | assigns: assigns, prints: prints, components: components}}
  end

  defp view_template("counter"), do: "counter"
  defp view_template("withcomp"), do: "withcomp"
  defp view_template("twocomp"), do: "twocomp"
  defp view_template(other), do: throw({:unknown_view, other})

  # Mirror of the port's Counter.Mount: `start` is used only when it is an
  # integer, `label` defaults to "Counter".
  defp mount_assigns("counter", params) do
    start = if is_integer(params["start"]), do: params["start"], else: 0
    label = if is_binary(params["label"]) and params["label"] != "", do: params["label"], else: "Counter"
    %{count: start, label: label}
  end

  defp mount_assigns(view, params) when view in ["withcomp", "twocomp"] do
    %{
      title: if(is_binary(params["title"]), do: params["title"], else: "Title"),
      text: if(is_binary(params["text"]), do: params["text"], else: "hi")
    }
  end

  # Mirror of the port's Counter.HandleEvent. Upstream LiveView has no type
  # assertion here: whatever the client sent is assigned.
  defp handle_event(assigns, "inc", _payload),
    do: {Map.update!(assigns, :count, &(&1 + 1)), [:count]}

  defp handle_event(assigns, "dec", _payload),
    do: {Map.update!(assigns, :count, &(&1 - 1)), [:count]}

  defp handle_event(assigns, "set", payload) do
    case Map.fetch(payload, "value") do
      {:ok, value} -> {Map.put(assigns, :count, value), [:count]}
      :error -> {assigns, []}
    end
  end

  defp handle_event(assigns, "label", payload) do
    case Map.fetch(payload, "text") do
      {:ok, text} when is_binary(text) -> {Map.put(assigns, :label, text), [:label]}
      _ -> {assigns, []}
    end
  end

  defp handle_event(assigns, "title", payload) do
    case Map.fetch(payload, "text") do
      {:ok, text} when is_binary(text) -> {Map.put(assigns, :title, text), [:title]}
      _ -> {assigns, []}
    end
  end

  defp handle_event(assigns, _unknown, _payload), do: {assigns, []}

  # ------------------------------------------------------------- normalising ----

  # canon turns an upstream diff into the shape both runners agree to emit:
  #
  #   * template dedup is undone — `%{s: 3, p: %{3 => [...]}}` becomes the literal
  #     statics list, and `:p` is dropped. The Go port has no template-dedup
  #     optimisation, so leaving it in would make every case fail on the same
  #     artefact instead of on the structure being compared.
  #   * component static cross-references inside `"c"` are resolved the way the
  #     JS client resolves them (an integer `:s` in a component diff points at
  #     another cid's statics).
  #   * `:r` (the "this is a root" marker) is dropped: it carries no structure.
  #   * every remaining key becomes a string; integer slot keys become decimal
  #     strings. `:c`, `:s`, `:k`, `:kc` keep their upstream names.
  #   * cids are renumbered in order of first appearance, starting at 1.
  #   * objects are emitted key-sorted.
  defp canon(diff, resolve? \\ true) do
    {components, diff} = Map.pop(diff, :c)
    components = if resolve?, do: resolve_xrefs(components || %{}), else: components || %{}

    cid_map = cid_order(diff, components)

    body = walk(Map.drop(diff, [:p, :r]), Map.get(diff, :p), cid_map)

    body =
      if map_size(components) == 0 do
        body
      else
        cdiffs =
          for {cid, cdiff} <- components, into: %{} do
            {Integer.to_string(Map.fetch!(cid_map, cid)),
             component_diff(cdiff, cid_map)}
          end

        Map.put(body, "c", cdiffs)
      end

    order(body)
  end

  # order recursively replaces every plain map with a key-sorted
  # Jason.OrderedObject, so the emitted JSON has sorted keys as HARNESS.md
  # requires.
  defp order(%{} = map) when not is_struct(map) do
    map
    |> Enum.map(fn {k, v} -> {to_string(k), order(v)} end)
    |> Enum.sort_by(&elem(&1, 0))
    |> Jason.OrderedObject.new()
  end

  defp order(list) when is_list(list), do: Enum.map(list, &order/1)
  defp order(other), do: other

  # Inside `"c"`, an integer `"s"` is a pointer at another cid's statics, not a
  # template index, so it is remapped as a cid (keeping its sign).
  defp component_diff(cdiff, cid_map) do
    case Map.fetch(cdiff, :s) do
      {:ok, ref} when is_integer(ref) ->
        mapped = Map.get(cid_map, abs(ref), abs(ref))
        mapped = if ref < 0, do: -mapped, else: mapped

        cdiff
        |> Map.drop([:s, :p, :r])
        |> walk(Map.get(cdiff, :p), cid_map)
        |> Map.put("s", mapped)

      _ ->
        walk(Map.drop(cdiff, [:p, :r]), Map.get(cdiff, :p), cid_map)
    end
  end

  # Resolve `%{s: other_cid}` component diffs into standalone diffs, exactly as
  # Phoenix.LiveView.Diff.to_iodata/2 does for the client.
  defp resolve_xrefs(components) do
    Enum.reduce(Map.keys(components), components, &resolve_xref(&1, &2))
  end

  defp resolve_xref(cid, components) do
    case components[cid] do
      %{s: static} = diff when is_integer(static) ->
        target = abs(static)
        components = resolve_xref(target, components)
        Map.put(components, cid, deep_merge(components[target], Map.delete(diff, :s)))

      _ ->
        components
    end
  end

  defp deep_merge(original, extra) do
    Map.merge(original, extra, fn
      _k, %{} = a, %{} = b when not is_struct(a) and not is_struct(b) -> deep_merge(a, b)
      _k, _a, b -> b
    end)
  end

  # cids in order of first appearance: the parent diff first (slot order), then
  # any cid only reachable from another component.
  defp cid_order(diff, components) do
    found = collect_cids(diff, []) ++ Enum.sort(Map.keys(components))

    found
    |> Enum.uniq()
    |> Enum.filter(&Map.has_key?(components, &1))
    |> Enum.with_index(1)
    |> Map.new()
  end

  defp collect_cids(%{} = map, acc) when not is_struct(map) do
    map
    |> Enum.sort_by(fn {k, _} -> sort_key(k) end)
    |> Enum.reduce(acc, fn {k, v}, acc ->
      if k in [:s, :p, :r, :kc, :t, :e, :stream], do: acc, else: collect_cids(v, acc)
    end)
  end

  defp collect_cids(list, acc) when is_list(list),
    do: Enum.reduce(list, acc, &collect_cids/2)

  defp collect_cids(cid, acc) when is_integer(cid), do: acc ++ [cid]
  defp collect_cids(_other, acc), do: acc

  # walk rewrites one diff node. `template` is the enclosing `:p` map, needed to
  # expand an integer `:s`.
  defp walk(%{} = map, template, cid_map) when not is_struct(map) do
    template = Map.get(map, :p) || template

    map
    |> Map.drop([:p, :r])
    |> Enum.map(fn
      {:s, static} when is_integer(static) ->
        {"s", Map.fetch!(template, static)}

      {:s, static} ->
        {"s", static}

      {:k, keyed} ->
        {"k", walk(keyed, template, cid_map)}

      {:kc, count} ->
        {"kc", count}

      {:stream, stream} ->
        {"stream", stringify(stream)}

      {key, value} ->
        {key_name(key), walk(value, template, cid_map)}
    end)
    |> Map.new()
  end

  defp walk(cid, _template, cid_map) when is_integer(cid), do: Map.get(cid_map, cid, cid)
  defp walk(list, template, cid_map) when is_list(list), do: Enum.map(list, &walk(&1, template, cid_map))
  defp walk(other, _template, _cid_map), do: other

  defp key_name(k) when is_integer(k), do: Integer.to_string(k)
  defp key_name(k) when is_atom(k), do: Atom.to_string(k)
  defp key_name(k) when is_binary(k), do: k

  defp sort_key(k) when is_integer(k), do: {0, k}
  defp sort_key(k), do: {1, to_string(k)}

  defp stringify(%{} = map) when not is_struct(map),
    do: map |> Enum.map(fn {k, v} -> {key_name(k), stringify(v)} end) |> Map.new()

  defp stringify(list) when is_list(list), do: Enum.map(list, &stringify/1)
  defp stringify(other), do: other

  # ------------------------------------------------------------------ input ----

  defp atomise(nil), do: %{}

  defp atomise(map) when is_map(map) do
    Map.new(map, fn {k, v} -> {to_atom(k), atomise_value(k, v)} end)
  end

  # The `inner` assign of the "nest" template is a nested sub-template, which
  # HEEx expresses as a function-component call, so it needs no value here; the
  # "v" assign feeds it instead.
  defp atomise_value(_k, v), do: v

  defp to_atom(k) when is_atom(k), do: k

  defp to_atom(k) when is_binary(k) do
    if k in @assign_keys, do: String.to_existing_atom(k), else: throw({:unknown_assign, k})
  end
end
