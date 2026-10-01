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
	info.Name = strings.Join(doc.Strings("name"), ", ")
	info.Description = strings.Join(doc.Strings("description"), ", ")
	info.Version = strings.Join(doc.Strings("version"), ", ")
	info.CodeRepository = doc.Strings("codeRepository")
	info.Licenses = doc.Strings("license")
	info.Keywords = doc.Strings("keywords")
	info.ProgrammingLanguages = doc.Strings("programmingLanguage")
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

func codemetaAuthors(agents []codemeta.Agent) []brief.CodemetaAuthor {
	authors := make([]brief.CodemetaAuthor, 0, len(agents))
	for _, agent := range agents {
		author := codemetaAuthor(agent)
		if agent.Kind() == codemeta.AgentRole {
			nested := codemetaAuthors(agent.Agents())
			for _, member := range nested {
				if member.Role == "" {
					member.Role = author.Role
				}
				authors = append(authors, member)
			}
			if len(nested) != 0 {
				continue
			}
		}
		authors = append(authors, author)
	}
	return authors
}

func codemetaAuthor(agent codemeta.Agent) brief.CodemetaAuthor {
	return brief.CodemetaAuthor{
		Name:       codemetaAuthorName(agent),
		Identifier: agent.Identifier().Text(),
		GivenName:  strings.Join(agent.Strings("givenName"), " "),
		FamilyName: strings.Join(agent.Strings("familyName"), " "),
		Role:       strings.Join(agent.Strings("roleName"), ", "),
		Kind:       codemetaAgentKind(agent.Kind()),
	}
}

func codemetaAuthorName(agent codemeta.Agent) string {
	if agent.Kind() == codemeta.AgentText {
		return agent.Name()
	}
	if name := strings.Join(agent.Strings("name"), ", "); name != "" {
		return name
	}
	names := append(agent.Strings("givenName"), agent.Strings("familyName")...)
	return strings.Join(names, " ")
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
