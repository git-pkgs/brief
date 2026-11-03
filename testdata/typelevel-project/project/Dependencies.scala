import sbt._

object Dependencies {
  val all = Seq(
    "org.typelevel" %% "cats-effect" % "3.6.1",
    "co.fs2" %% "fs2-core" % "3.12.0",
    "org.http4s" %% "http4s-ember-server" % "0.23.30",
    "io.circe" %% "circe-generic" % "0.14.13",
    "org.tpolecat" %% "doobie-core" % "1.0.0-RC9",
    "org.typelevel" %% "weaver-cats" % "0.9.0" % Test,
    "org.scalacheck" %% "scalacheck" % "1.18.1" % Test
  )
}
