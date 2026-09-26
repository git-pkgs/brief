package detect

import (
	"errors"
	"strings"

	"github.com/git-pkgs/brief"
	"github.com/git-pkgs/codemeta"
)

const codemetaByteLimit = 1 << 20

func (e *Engine) detectCodemeta(path string) *brief.CodemetaInfo {
	info := &brief.CodemetaInfo{Path: path}
	data, err := e.safeReadFileLimit(path, codemetaByteLimit+1)
	if err != nil {
		info.ParseStatus = "read_error"
		info.Diagnostics = []brief.CodemetaDiagnostic{{Code: "read_error", Message: err.Error()}}
		return info
	}
	doc, err := codemeta.ParseWithOptions(data, codemeta.ParseOptions{MaxBytes: codemetaByteLimit})
	if err != nil {
		info.ParseStatus = "syntax_error"
		switch {
		case errors.Is(err, codemeta.ErrLimit):
			info.ParseStatus = "limit_exceeded"
		case errors.Is(err, codemeta.ErrUnsupported):
			info.ParseStatus = "unsupported_syntax"
		case errors.Is(err, codemeta.ErrType):
			info.ParseStatus = "type_error"
		}
		var problem *codemeta.Error
		if errors.As(err, &problem) {
			info.Diagnostics = []brief.CodemetaDiagnostic{codemetaDiagnostic(problem.Diagnostic)}
		}
		return info
	}
	info.ParseStatus = "parsed"
	info.ValidationStatus = "valid"
	info.ContextVersion = string(doc.Version())
	info.Name = doc.Name()
	info.Description = doc.Description()
	info.Version = doc.SoftwareVersion().Text()
	info.CodeRepository = codemetaStrings(doc.CodeRepository())
	info.Licenses = codemetaStrings(doc.License())
	info.Keywords = codemetaStrings(doc.Keywords())
	info.ProgrammingLanguages = codemetaStrings(doc.ProgrammingLanguages())
	info.Authors = codemetaAuthors(doc.Author())
	for _, issue := range doc.Validate() {
		if info.ValidationStatus != "unsupported_version" {
			info.ValidationStatus = "invalid"
		}
		if issue.Code == "unsupported_version" {
			info.ValidationStatus = "unsupported_version"
		}
		info.Diagnostics = append(info.Diagnostics, codemetaDiagnostic(issue))
	}
	return info
}

func codemetaDiagnostic(issue codemeta.Diagnostic) brief.CodemetaDiagnostic {
	return brief.CodemetaDiagnostic{
		Code: issue.Code, Path: issue.Path, Message: issue.Message,
		Line: issue.Line, Column: issue.Column,
	}
}

func codemetaStrings(value codemeta.Value) []string {
	var values []string
	for _, item := range value.Values() {
		text := ""
		if item.Kind() == codemeta.String {
			text = item.Text()
		} else if item.Kind() == codemeta.Object {
			text = item.Get("@id").Text()
			if text == "" {
				text = item.Get("name").Text()
			}
		}
		if text != "" {
			values = append(values, text)
		}
	}
	return values
}

func codemetaAuthors(agents []codemeta.Agent) []brief.CodemetaAuthor {
	authors := make([]brief.CodemetaAuthor, 0, len(agents))
	for _, agent := range agents {
		author := codemetaAuthor(agent)
		if agent.Kind() == codemeta.AgentRole {
			for _, nested := range agent.Agents() {
				if author.Name == "" {
					author.Name = codemetaAuthorName(nested)
				}
			}
		}
		authors = append(authors, author)
	}
	return authors
}

func codemetaAuthor(agent codemeta.Agent) brief.CodemetaAuthor {
	return brief.CodemetaAuthor{
		Name:       codemetaAuthorName(agent),
		GivenName:  agent.GivenName(),
		FamilyName: agent.FamilyName(),
		Role:       agent.RoleName().Text(),
		Kind:       codemetaAgentKind(agent.Kind()),
	}
}

func codemetaAuthorName(agent codemeta.Agent) string {
	if name := agent.Name(); name != "" {
		return name
	}
	return strings.TrimSpace(agent.GivenName() + " " + agent.FamilyName())
}

func codemetaAgentKind(kind codemeta.AgentKind) string {
	switch kind {
	case codemeta.AgentText:
		return "text"
	case codemeta.AgentReference:
		return "reference"
	case codemeta.AgentPerson:
		return "person"
	case codemeta.AgentOrganization:
		return "organization"
	case codemeta.AgentRole:
		return "role"
	case codemeta.AgentConflict:
		return "conflict"
	default:
		return "unknown"
	}
}
