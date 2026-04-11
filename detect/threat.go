package detect

import (
	"sort"

	"github.com/git-pkgs/brief"
)

// ThreatModel resolves the threat surface implied by detected tools.
// Each tool's taxonomy tags are matched against the threat mappings in
// _threats.toml; matched threat IDs are unioned with any explicit
// [security].threats on the tool, then resolved against the registry.
func (e *Engine) ThreatModel(r *brief.Report) *brief.ThreatReport {
	tr := &brief.ThreatReport{
		Version: brief.Version,
		Path:    r.Path,
	}

	for eco := range e.detectedEcosystems {
		tr.Ecosystems = append(tr.Ecosystems, eco)
	}
	sort.Strings(tr.Ecosystems)

	// threat ID -> set of tool names that introduced it
	introducedBy := make(map[string]map[string]bool)
	// threat ID -> first matching mapping note (for context in output)
	notes := make(map[string]string)

	addThreat := func(id, tool, note string) {
		if introducedBy[id] == nil {
			introducedBy[id] = make(map[string]bool)
		}
		introducedBy[id][tool] = true
		if notes[id] == "" && note != "" {
			notes[id] = note
		}
	}

	contributes := e.resolveThreats(allDetections(r), addThreat)

	// Build stack from tools that actually contribute threats or sinks.
	for _, d := range allDetections(r) {
		if contributes[d.Name] {
			tr.Stack = append(tr.Stack, brief.StackEntry{
				Name:     d.Name,
				Taxonomy: d.Taxonomy,
			})
		}
	}

	// Resolve threat IDs against the registry and sort.
	ids := make([]string, 0, len(introducedBy))
	for id := range introducedBy {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		def := e.KB.Threats[id]
		introducers := make([]string, 0, len(introducedBy[id]))
		for name := range introducedBy[id] {
			introducers = append(introducers, name)
		}
		sort.Strings(introducers)

		tr.Threats = append(tr.Threats, brief.Threat{
			ID:           id,
			CWE:          def.CWE,
			OWASP:        def.OWASP,
			Title:        def.Title,
			IntroducedBy: introducers,
			Note:         notes[id],
		})
	}

	sort.Slice(tr.Stack, func(i, j int) bool {
		return tr.Stack[i].Name < tr.Stack[j].Name
	})

	return tr
}

// Sinks collects known dangerous functions from all detected tools.
// CWE is filled from the threat registry when the sink doesn't carry one.
func (e *Engine) Sinks(r *brief.Report) *brief.SinkReport {
	sr := &brief.SinkReport{
		Version: brief.Version,
		Path:    r.Path,
	}

	for _, d := range allDetections(r) {
		tool := e.KB.ByName[d.Name]
		if tool == nil {
			continue
		}
		for _, s := range tool.Security.Sinks {
			cwe := s.CWE
			if cwe == "" {
				cwe = e.KB.Threats[s.Threat].CWE
			}
			sr.Sinks = append(sr.Sinks, brief.SinkEntry{
				Symbol: s.Symbol,
				Tool:   d.Name,
				Threat: s.Threat,
				CWE:    cwe,
				Note:   s.Note,
			})
		}
	}

	sort.Slice(sr.Sinks, func(i, j int) bool {
		if sr.Sinks[i].Tool != sr.Sinks[j].Tool {
			return sr.Sinks[i].Tool < sr.Sinks[j].Tool
		}
		return sr.Sinks[i].Symbol < sr.Sinks[j].Symbol
	})

	return sr
}

// allDetections flattens languages, package managers, and tools into one slice.
func allDetections(r *brief.Report) []brief.Detection {
	var all []brief.Detection
	all = append(all, r.Languages...)
	all = append(all, r.PackageManagers...)
	for _, cat := range sortedKeys(r.Tools) {
		all = append(all, r.Tools[cat]...)
	}
	return all
}

// resolveThreats iterates detections, matches taxonomy tags against threat
// mappings, and calls addThreat for each hit. Returns a set of tool names
// that contribute threats or sinks (for stack filtering).
func (e *Engine) resolveThreats(detections []brief.Detection, addThreat func(id, tool, note string)) map[string]bool {
	contributes := make(map[string]bool)
	for _, d := range detections {
		tool := e.KB.ByName[d.Name]
		if tool == nil {
			continue
		}

		tags := make(map[string]bool)
		for _, t := range tool.Taxonomy.Tags() {
			tags[t] = true
		}

		for _, m := range e.KB.ThreatMappings {
			if matchesAll(tags, m.Match) {
				contributes[d.Name] = true
				for _, id := range m.Threats {
					addThreat(id, d.Name, m.Note)
				}
			}
		}

		for _, id := range tool.Security.Threats {
			contributes[d.Name] = true
			addThreat(id, d.Name, "")
		}

		if len(tool.Security.Sinks) > 0 {
			contributes[d.Name] = true
		}
	}
	return contributes
}

// matchesAll reports whether the tag set contains every required tag.
// An empty required slice matches nothing (vacuous mappings shouldn't fire).
func matchesAll(have map[string]bool, required []string) bool {
	if len(required) == 0 {
		return false
	}
	for _, r := range required {
		if !have[r] {
			return false
		}
	}
	return true
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
