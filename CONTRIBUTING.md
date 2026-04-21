# Contributing to brief

The easiest way to contribute is adding tool definitions. Each tool is a single TOML file in the `knowledge/` directory. No Go code changes required.

## Adding a tool

Create a TOML file in the appropriate ecosystem directory:

```
knowledge/ruby/my-tool.toml
```

Every tool definition has four required sections plus two optional:

```toml
[tool]
name = "My Tool"
category = "lint"                    # see categories below
homepage = "https://example.com"
docs = "https://example.com/docs"
repo = "https://github.com/org/repo"
description = "One line description"

[detect]
files = [".my-tool.yml"]            # file or directory presence
dependencies = ["my-tool"]          # package name in manifest
ecosystems = ["ruby"]               # which language ecosystems this belongs to

[commands]
run = "bundle exec my-tool"
alternatives = ["my-tool"]

[config]
files = [".my-tool.yml"]

[taxonomy]                          # optional, oss-taxonomy term IDs
role = ["framework"]
function = ["api-development"]
layer = ["backend"]

[[security.sinks]]                  # optional, dangerous functions
symbol = "html_safe"
threat = "xss"                      # must be a threat ID from _threats.toml
```

The `[taxonomy]` values are kebab-case term IDs from [oss-taxonomy](https://github.com/ecosyste-ms/oss-taxonomy). The test suite validates them against a vendored copy of the term list at `kb/testdata/oss-taxonomy-terms.txt` — refresh it with `curl -s https://taxonomy.ecosyste.ms/terms.txt -o kb/testdata/oss-taxonomy-terms.txt` when oss-taxonomy adds terms.

## Detection methods

brief has five detection primitives. A tool matches when any of its primitives match. The more specific the primitive, the higher the confidence.

### files (medium/low confidence)

Checks if files or directories exist. Directories end with `/`. Supports glob patterns.

```toml
[detect]
files = [".rspec", "spec/", "*.gemspec"]
```

A specific config file like `.rspec` gives medium confidence. A generic directory like `spec/` gives low confidence.

### dependencies (high confidence)

Checks if a package name appears in the project's parsed manifests. Uses the git-pkgs/manifests library for proper parsing with scope awareness.

```toml
[detect]
dependencies = ["rspec", "rspec-core"]
```

Matches dependencies in any scope (runtime, dev, test). Use `dev_dependencies` to match only development/test/build scoped packages:

```toml
[detect]
dev_dependencies = ["rubocop"]
```

### file_contains (high confidence)

Checks if a file contains a specific string. Useful for tools configured inside shared config files like `pyproject.toml`.

```toml
[detect.file_contains]
"pyproject.toml" = ["[tool.ruff]"]
```

### key_exists (medium confidence)

Checks if a dot-separated key path exists in a JSON or TOML file. Useful for checking if a tool has configuration in a structured file.

```toml
[detect.key_exists]
"package.json" = ["jest", "scripts.test"]
```

### ecosystems

Declares which language ecosystems the tool belongs to. Tools with ecosystems are only matched when that language is detected in the project, preventing false positives like ExUnit matching `test/` in a JavaScript project.

```toml
[detect]
ecosystems = ["ruby"]
```

Shared tools that work across languages (Docker, GitHub Actions, Dependabot) should not set ecosystems.

## Categories

| Category | What it means |
| --- | --- |
| `language` | Programming language |
| `package_manager` | Dependency management |
| `test` | Test framework or runner |
| `lint` | Linter or static analysis |
| `format` | Code formatter |
| `typecheck` | Type checker |
| `build` | Build tool |
| `docs` | Documentation generator |
| `security` | Security scanner |
| `ci` | Continuous integration |
| `container` | Container tooling |
| `dependency_bot` | Automated dependency updates |

## Adding an ecosystem

Create a directory under `knowledge/` and add at least:

1. `language.toml` with the language detection rules
2. A package manager
3. A test framework
4. A linter

## Lockfile reporting

Package managers can declare their lockfile:

```toml
[config]
files = ["Gemfile", "Gemfile.lock"]
lockfile = "Gemfile.lock"
```

## Testing

Add a minimal project fixture in `testdata/` and run:

```
go test ./...
```

The knowledge base is validated on every test run. Malformed TOML will fail the build.

## Shared configuration

Files prefixed with `_` in ecosystem directories are shared config, not tool definitions:

- `_shared/_scripts.toml` - script source definitions (Makefile, package.json)
- `_shared/_resources.toml` - repository document patterns (README, LICENSE, CODEOWNERS, FUNDING, etc.)
- `_shared/_layout.toml` - source and test directory patterns
- `_shared/_style.toml` - style config files and inference settings
- `_shared/_runtimes.toml` - runtime version file patterns
- `_shared/_manifests.toml` - manifest file list for dependency detection
- `_shared/_ci.toml` - CI matrix parsing configuration

A resource entry in `_resources.toml` looks like this:

```toml
[[resources]]
[resources.resource]
name = "Contributing"
field = "contributing"
group = "community"
patterns = ["contributing", "contributing.md", "contributing.txt", "contributing.rst"]
dirs = ["docs", ".github", ".gitlab"]
```

`field` is the JSON key the path is written to. `group` places it under one of `legal`, `community`, `security`, or `metadata`; omit it for top-level fields like `readme` and `license`. `patterns` are matched case-insensitively against directory listings, so list each pattern once in lowercase. List explicit extensions rather than a trailing glob for prose files so `support.md` matches but `docs/Support-Tiers.md` does not. `dirs` lists extra directories to search after the repo root; root always wins on a tie.
