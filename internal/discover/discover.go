package discover

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/example/yocto-lens/internal/model"
)

var assign = regexp.MustCompile(`^([A-Za-z0-9_:+${}.-]+)\s*(\?=|:=|=|\+=|\. =|\.\=)\s*"(.*)"`)

var skipDirs = map[string]bool{
	".git": true, ".repo": true, "tmp": true, "work": true, "deploy": true,
	"downloads": true, "sstate-cache": true, "sysroots": true, "cache": true,
	"node_modules": true, "__pycache__": true,
}

func LayersFromInputs(inputs []string) ([]model.Layer, error) {
	seen := map[string]bool{}
	var layers []model.Layer
	for _, input := range inputs {
		abs, err := filepath.Abs(input)
		if err != nil {
			return nil, err
		}
		found, err := discoverUnder(abs)
		if err != nil {
			return nil, err
		}
		for _, l := range found {
			if !seen[l.Path] {
				seen[l.Path] = true
				layers = append(layers, l)
			}
		}
	}
	return layers, nil
}

func discoverUnder(root string) ([]model.Layer, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		root = filepath.Dir(root)
	}
	if isLayer(root) {
		l, err := parseLayer(root)
		if err != nil {
			return nil, err
		}
		return []model.Layer{l}, nil
	}
	var layers []model.Layer
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if skipDirs[name] {
			return filepath.SkipDir
		}
		if isLayer(path) {
			l, err := parseLayer(path)
			if err == nil {
				layers = append(layers, l)
			}
			return filepath.SkipDir
		}
		return nil
	})
	return layers, err
}

func isLayer(path string) bool {
	st, err := os.Stat(filepath.Join(path, "conf", "layer.conf"))
	return err == nil && !st.IsDir()
}

func parseLayer(path string) (model.Layer, error) {
	l := model.Layer{Name: filepath.Base(path), Path: path}
	file, err := os.Open(filepath.Join(path, "conf", "layer.conf"))
	if err != nil {
		return l, err
	}
	defer file.Close()
	s := bufio.NewScanner(file)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		m := assign.FindStringSubmatch(line)
		if len(m) != 4 {
			continue
		}
		key, val := m[1], m[3]
		switch {
		case strings.HasPrefix(key, "BBFILE_COLLECTIONS"):
			// Name is commonly the collection token. Keep directory name if expanded variables hide it.
		case strings.HasPrefix(key, "BBFILE_PRIORITY"):
			l.Priority = val
		case key == "LAYERSERIES_COMPAT" || strings.HasPrefix(key, "LAYERSERIES_COMPAT_") || strings.HasPrefix(key, "LAYERSERIES_COMPAT:"):
			l.Series = val
		case strings.HasPrefix(key, "BBFILES"):
			l.BBFiles = val
		}
	}
	return l, s.Err()
}

func ShouldSkipDir(name string) bool { return skipDirs[name] }
