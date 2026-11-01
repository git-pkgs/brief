package detect

import (
	"slices"
	"testing"
)

func TestScalaToolFixtures(t *testing.T) {
	for _, tt := range []struct {
		fixture        string
		packageManager string
		tools          map[string][]string
	}{
		{"mill-project", "Mill", map[string][]string{"test": {"MUnit"}}},
		{"scala-cli-project", "Scala CLI", map[string][]string{"test": {"utest"}}},
		{"sbt-plugins-project", "sbt", map[string][]string{"test": {"specs2"}, "lint": {"Scalafix", "Scapegoat"}}},
	} {
		t.Run(tt.fixture, func(t *testing.T) {
			report := runOn(t, "../testdata/"+tt.fixture)
			if names := packageManagerNames(report); !slices.Contains(names, tt.packageManager) {
				t.Errorf("package managers = %v, want %s", names, tt.packageManager)
			}
			for category, tools := range tt.tools {
				for _, tool := range tools {
					assertHighConfidenceToolDetected(t, report, category, tool)
				}
			}
		})
	}
}

func TestScalaPackageManagerSignals(t *testing.T) {
	for _, tt := range []struct{ name, file, content, packageManager string }{
		{"Mill build.mill", "build.mill", "object app extends ScalaModule\n", "Mill"},
		{"Mill build.sc", "build.sc", "object app extends ScalaModule\n", "Mill"},
		{"Mill version file", ".mill-version", "0.12.14\n", "Mill"},
		{"Scala CLI project file", "project.scala", "//> using scala 3.7.1\n", "Scala CLI"},
		{"sbt alternate root file", "settings.sbt", "name := \"example\"\n", "sbt"},
		{"sbt build properties", "project/build.properties", "sbt.version=1.11.2\n", "sbt"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "src/Main.scala", "object Main\n")
			writeFile(t, dir, tt.file, tt.content)
			if names := packageManagerNames(runOn(t, dir)); !slices.Contains(names, tt.packageManager) {
				t.Errorf("package managers = %v, want %s", names, tt.packageManager)
			}
		})
	}
}

func TestScalaTestAndLintSignals(t *testing.T) {
	for _, tt := range []struct{ category, tool, file, content string }{
		{"test", "MUnit", "build.sbt", "libraryDependencies += \"org.scalameta\" %% \"munit\" % \"1.1.1\" % Test\n"},
		{"test", "MUnit", "build.mill", "def mvnDeps = Seq(mvn\"org.scalameta::munit::1.1.1\")\n"},
		{"test", "MUnit", "build.sc", "object test extends ScalaTests with TestModule.Munit\n"},
		{"test", "MUnit", "project.scala", "//> using test.dep org.scalameta::munit::1.1.1\n"},
		{"test", "specs2", "build.sbt", "libraryDependencies += \"org.specs2\" %% \"specs2-core\" % \"4.21.0\" % Test\n"},
		{"test", "specs2", "build.mill", "def ivyDeps = Agg(ivy\"org.specs2::specs2-core:4.21.0\")\n"},
		{"test", "utest", "build.sbt", "libraryDependencies += \"com.lihaoyi\" %% \"utest\" % \"0.8.9\" % Test\n"},
		{"test", "utest", "build.mill", "object test extends ScalaTests with TestModule.Utest\n"},
		{"test", "utest", "project/Dependencies.scala", "val utest = \"com.lihaoyi\" %% \"utest\" % \"0.8.9\"\n"},
		{"lint", "Scalafix", ".scalafix.conf", "rules = [OrganizeImports]\n"},
		{"lint", "Scalafix", "project/plugins.sbt", "addSbtPlugin(\"ch.epfl.scala\" % \"sbt-scalafix\" % \"0.14.3\")\n"},
		{"lint", "Scapegoat", "build.sbt", "ThisBuild / scapegoatVersion := \"3.1.4\"\n"},
		{"lint", "Scapegoat", "project/plugins.sbt", "addSbtPlugin(\"com.sksamuel.scapegoat\" %% \"sbt-scapegoat\" % \"1.2.13\")\n"},
	} {
		t.Run(tt.tool+"/"+tt.file, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "src/Main.scala", "object Main\n")
			writeFile(t, dir, tt.file, tt.content)
			assertToolDetected(t, runOn(t, dir), tt.category, tt.tool)
		})
	}
}

func TestScalaToolNearMiss(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src/Main.scala", "object Main\n")
	writeFile(t, dir, "build.sbt", "// community plugins: munit, specs2 and utest are not used here\n"+
		"libraryDependencies += \"org.scalatest\" %% \"scalatest\" % \"3.2.19\" % Test\n"+
		"libraryDependencies += \"org.scalameta\" %% \"munit-diff\" % \"1.1.1\"\n")
	report := runOn(t, dir)
	for _, tool := range []string{"MUnit", "specs2", "utest"} {
		assertToolNotDetected(t, report, "test", tool)
	}
	assertToolDetected(t, report, "test", "ScalaTest")
	for _, name := range packageManagerNames(report) {
		if name == "Mill" || name == "Scala CLI" {
			t.Errorf("unexpected package manager %s", name)
		}
	}
}
