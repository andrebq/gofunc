package funcs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func findBinaries(root string) ([]string, error) {
	var bins []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(info.Name(), ".out") {
			bins = append(bins, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk dir: %w", err)
	}
	return bins, nil
}

func LoadFuncs(bindir string) ([]*Func, error) {
	binlist, err := findBinaries(bindir)
	if err != nil {
		return nil, err
	}
	var funcs []*Func
	for _, f := range binlist {
		bn := filepath.Base(f)
		if !strings.HasSuffix(bn, ".out") {
			continue
		}
		info, err := os.Stat(f)
		if err != nil {
			return nil, fmt.Errorf("get file info: %w", err)
		}
		const execMode = 0111
		if info.Mode().Perm()&execMode == 0 {
			continue
		}
		fun := &Func{
			binfile: f,
		}
		funcs = append(funcs, fun)
	}
	return funcs, nil
}
