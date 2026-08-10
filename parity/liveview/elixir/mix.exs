defmodule LiveViewParity.MixProject do
  use Mix.Project

  def project do
    [
      app: :liveview_parity,
      version: "0.1.0",
      elixir: "~> 1.14",
      start_permanent: false,
      deps: deps(),
      escript: [main_module: LiveViewParity.Runner]
    ]
  end

  def application do
    [extra_applications: [:logger]]
  end

  defp deps do
    [
      {:phoenix_live_view, "~> 1.0"},
      {:jason, "~> 1.4"}
    ]
  end
end
