defmodule ObanParity.MixProject do
  use Mix.Project

  def project do
    [
      app: :oban_parity,
      version: "0.1.0",
      elixir: "~> 1.14",
      start_permanent: false,
      deps: deps(),
      escript: [main_module: ObanParity.Runner]
    ]
  end

  def application do
    [extra_applications: [:logger]]
  end

  defp deps do
    [
      {:oban, "~> 2.18"},
      {:jason, "~> 1.4"}
    ]
  end
end
