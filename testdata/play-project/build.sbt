lazy val root = (project in file("."))
  .enablePlugins(PlayScala)
  .settings(
    name := "play-project",
    scalaVersion := "3.3.6",
    libraryDependencies += guice
  )
