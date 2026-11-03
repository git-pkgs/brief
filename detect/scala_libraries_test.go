package detect

import "testing"

func TestScalaLibraryFixtures(t *testing.T) {
	for _, tt := range []struct {
		fixture string
		tools   map[string][]string
	}{
		{"play-project", map[string][]string{"library": {"Play"}}},
		{"typelevel-project", map[string][]string{
			"library":        {"Cats Effect", "fs2", "http4s", "Circe", "Doobie"},
			"test":           {"Weaver", "ScalaCheck"},
			"build":          {"Bloop"},
			"dependency_bot": {"Scala Steward"},
		}},
	} {
		t.Run(tt.fixture, func(t *testing.T) {
			report := runOn(t, "../testdata/"+tt.fixture)
			for category, tools := range tt.tools {
				for _, tool := range tools {
					assertToolDetected(t, report, category, tool)
				}
			}
		})
	}
}

func TestScalaLibrarySignals(t *testing.T) {
	for _, tt := range []struct{ category, tool, file, content string }{
		{"library", "Play", "build.sbt", "lazy val root = (project in file(\".\")).enablePlugins(PlayScala)\n"},
		{"library", "Play", "build.sbt", "lazy val root = (project in file(\".\")).enablePlugins(PlayJava)\n"},
		{"library", "Play", "project/plugins.sbt", "addSbtPlugin(\"com.typesafe.play\" % \"sbt-plugin\" % \"2.9.8\")\n"},
		{"library", "Play", "build.mill", "import mill.playlib._\nobject app extends PlayModule\n"},
		{"library", "Akka HTTP", "build.sbt", "libraryDependencies += \"com.typesafe.akka\" %% \"akka-http\" % \"10.5.3\"\n"},
		{"library", "Akka HTTP", "build.mill", "def mvnDeps = Seq(mvn\"com.typesafe.akka::akka-http:10.5.3\")\n"},
		{"library", "Apache Pekko", "build.sbt", "libraryDependencies += \"org.apache.pekko\" %% \"pekko-actor-typed\" % \"1.1.5\"\n"},
		{"library", "Apache Pekko", "build.mill", "def mvnDeps = Seq(mvn\"org.apache.pekko::pekko-stream:1.1.5\")\n"},
		{"library", "Apache Pekko HTTP", "build.sbt", "libraryDependencies += \"org.apache.pekko\" %% \"pekko-http\" % \"1.2.0\"\n"},
		{"library", "Apache Pekko HTTP", "project.scala", "//> using dep org.apache.pekko::pekko-http:1.2.0\n"},
		{"library", "http4s", "build.sbt", "libraryDependencies += \"org.http4s\" %% \"http4s-ember-server\" % \"0.23.30\"\n"},
		{"library", "http4s", "project.scala", "//> using dep org.http4s::http4s-ember-server:0.23.30\n"},
		{"library", "ZIO HTTP", "build.sbt", "libraryDependencies += \"dev.zio\" %% \"zio-http\" % \"3.3.3\"\n"},
		{"library", "ZIO HTTP", "build.mill", "def mvnDeps = Seq(mvn\"dev.zio::zio-http:3.3.3\")\n"},
		{"library", "Tapir", "build.sbt", "libraryDependencies += \"com.softwaremill.sttp.tapir\" %% \"tapir-core\" % \"1.11.35\"\n"},
		{"library", "Tapir", "build.sc", "def ivyDeps = Agg(ivy\"com.softwaremill.sttp.tapir::tapir-core:1.11.35\")\n"},
		{"library", "Finagle", "build.sbt", "libraryDependencies += \"com.twitter\" %% \"finagle-http\" % \"24.2.0\"\n"},
		{"library", "Finagle", "build.mill", "def mvnDeps = Seq(mvn\"com.twitter::finagle-http:24.2.0\")\n"},
		{"library", "Finatra", "build.sbt", "libraryDependencies += \"com.twitter\" %% \"finatra-http-server\" % \"24.2.0\"\n"},
		{"library", "Finatra", "build.mill", "def mvnDeps = Seq(mvn\"com.twitter::finatra-http-server:24.2.0\")\n"},
		{"library", "Akka", "build.sbt", "libraryDependencies += \"com.typesafe.akka\" %% \"akka-actor-typed\" % \"2.8.8\"\n"},
		{"library", "Akka", "build.sbt", "libraryDependencies += \"com.typesafe.akka\" %% \"akka-stream\" % \"2.8.8\"\n"},
		{"library", "Akka", "build.mill", "def mvnDeps = Seq(mvn\"com.typesafe.akka::akka-actor:2.8.8\")\n"},
		{"library", "Cats Effect", "build.sbt", "libraryDependencies += \"org.typelevel\" %% \"cats-effect\" % \"3.6.1\"\n"},
		{"library", "Cats Effect", "project.scala", "//> using dep org.typelevel::cats-effect:3.6.1\n"},
		{"library", "ZIO", "build.sbt", "libraryDependencies += \"dev.zio\" %% \"zio\" % \"2.1.19\"\n"},
		{"library", "ZIO", "build.mill", "def mvnDeps = Seq(mvn\"dev.zio::zio:2.1.19\")\n"},
		{"library", "fs2", "build.sbt", "libraryDependencies += \"co.fs2\" %% \"fs2-core\" % \"3.12.0\"\n"},
		{"library", "fs2", "build.mill", "def mvnDeps = Seq(mvn\"co.fs2::fs2-io:3.12.0\")\n"},
		{"library", "Monix", "build.sbt", "libraryDependencies += \"io.monix\" %% \"monix\" % \"3.4.1\"\n"},
		{"library", "Monix", "build.sc", "def ivyDeps = Agg(ivy\"io.monix::monix-eval:3.4.1\")\n"},
		{"library", "Apache Spark", "build.sbt", "libraryDependencies += \"org.apache.spark\" %% \"spark-sql\" % \"3.5.6\" % Provided\n"},
		{"library", "Apache Spark", "build.mill", "def mvnDeps = Seq(mvn\"org.apache.spark::spark-core:3.5.6\")\n"},
		{"library", "Apache Flink", "build.sbt", "libraryDependencies += \"org.apache.flink\" % \"flink-streaming-java\" % \"1.20.1\"\n"},
		{"library", "Apache Flink", "build.mill", "def mvnDeps = Seq(mvn\"org.apache.flink:flink-clients:1.20.1\")\n"},
		{"library", "Kafka Streams", "build.sbt", "libraryDependencies += \"org.apache.kafka\" %% \"kafka-streams-scala\" % \"3.9.1\"\n"},
		{"library", "Kafka Streams", "build.mill", "def mvnDeps = Seq(mvn\"org.apache.kafka:kafka-streams:3.9.1\")\n"},
		{"library", "Circe", "build.sbt", "libraryDependencies += \"io.circe\" %% \"circe-core\" % \"0.14.13\"\n"},
		{"library", "Circe", "project.scala", "//> using dep io.circe::circe-core:0.14.13\n"},
		{"library", "Slick", "build.sbt", "libraryDependencies += \"com.typesafe.slick\" %% \"slick\" % \"3.6.1\"\n"},
		{"library", "Slick", "build.mill", "def mvnDeps = Seq(mvn\"com.typesafe.slick::slick:3.6.1\")\n"},
		{"library", "Doobie", "project/Dependencies.scala", "val doobie = \"org.tpolecat\" %% \"doobie-core\" % \"1.0.0-RC9\"\n"},
		{"library", "Doobie", "build.mill", "def mvnDeps = Seq(mvn\"org.tpolecat::doobie-postgres:1.0.0-RC9\")\n"},
		{"library", "Quill", "build.sbt", "libraryDependencies += \"io.getquill\" %% \"quill-jdbc\" % \"4.8.6\"\n"},
		{"library", "Quill", "build.mill", "def mvnDeps = Seq(mvn\"io.getquill::quill-jdbc:4.8.6\")\n"},
		{"test", "Weaver", "build.sbt", "libraryDependencies += \"org.typelevel\" %% \"weaver-cats\" % \"0.9.0\" % Test\n"},
		{"test", "Weaver", "build.mill", "def mvnDeps = Seq(mvn\"com.disneystreaming::weaver-cats:0.8.4\")\n"},
		{"test", "ScalaCheck", "build.sbt", "libraryDependencies += \"org.scalacheck\" %% \"scalacheck\" % \"1.18.1\" % Test\n"},
		{"test", "ScalaCheck", "project.scala", "//> using test.dep org.scalacheck::scalacheck:1.18.1\n"},
		{"build", "Bloop", ".bsp/bloop.json", "{\"name\":\"Bloop\"}\n"},
		{"build", "Bloop", ".bloop/root.json", "{}\n"},
		{"dependency_bot", "Scala Steward", ".scala-steward.conf", "updates.limit = 5\n"},
		{"dependency_bot", "Scala Steward", ".github/.scala-steward.conf", "updates.limit = 5\n"},
		{"dependency_bot", "Scala Steward", ".config/.scala-steward.conf", "updates.limit = 5\n"},
	} {
		t.Run(tt.tool+"/"+tt.file, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "src/Main.scala", "object Main\n")
			writeFile(t, dir, tt.file, tt.content)
			assertToolDetected(t, runOn(t, dir), tt.category, tt.tool)
		})
	}
}

func TestScalaLibraryNearMiss(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src/Main.scala", "object Main\n")
	writeFile(t, dir, "build.sbt", "libraryDependencies ++= Seq(\n"+
		"  \"dev.zio\" %% \"zio-http\" % \"3.3.3\",\n"+
		"  \"org.playframework\" %% \"play-json\" % \"3.0.4\",\n"+
		"  \"org.typelevel\" %% \"munit-cats-effect\" % \"2.1.0\" % Test,\n"+
		"  \"com.typesafe.akka\" %% \"akka-http-spray-json\" % \"10.5.3\"\n"+
		")\n")
	report := runOn(t, dir)
	assertToolDetected(t, report, "library", "ZIO HTTP")
	for _, tool := range []string{"ZIO", "Play", "Cats Effect", "Akka HTTP", "Akka"} {
		assertToolNotDetected(t, report, "library", tool)
	}
}
