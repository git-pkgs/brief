//go:build !tinygo

package binary

import (
	"debug/buildinfo"
	"io"
)

func readGoBuild(r io.ReaderAt) (*GoBuild, error) {
	bi, err := buildinfo.Read(r)
	if err != nil {
		return nil, err
	}
	return goBuildFrom(bi), nil
}

func goBuildFrom(bi *buildinfo.BuildInfo) *GoBuild {
	g := &GoBuild{
		Version: bi.GoVersion,
		Path:    bi.Path,
	}
	if bi.Main.Path != "" {
		g.Main = bi.Main.Path + "@" + bi.Main.Version
	}
	for _, dep := range bi.Deps {
		if dep.Replace != nil {
			g.Deps = append(g.Deps, dep.Path+" => "+dep.Replace.Path+"@"+dep.Replace.Version)
		} else {
			g.Deps = append(g.Deps, dep.Path+"@"+dep.Version)
		}
	}
	return g
}
