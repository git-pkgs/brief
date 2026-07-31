defmodule RustlerProject.MixProject do
  use Mix.Project

  def project do
    [
      app: :rustler_project,
      version: "0.1.0",
      deps: deps()
    ]
  end

  def application do
    [extra_applications: [:logger]]
  end

  defp deps do
    [
      {:rustler, "~> 0.38"}
    ]
  end
end
