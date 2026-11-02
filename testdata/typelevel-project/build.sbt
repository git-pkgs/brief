import Dependencies._

lazy val root = (project in file("."))
  .settings(
    name := "typelevel-project",
    scalaVersion := "3.3.6",
    libraryDependencies ++= all
  )
