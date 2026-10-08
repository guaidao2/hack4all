package core

import (
	"strings"
	"testing"

	"github.com/guaidao2/hack4all/content"
)

const sampleMapBody = `---
id: demo-map
title_en: Demo Map
title_zh: 演示地图
---

<!-- lang:en -->
### The vertical axis: the stages

#### 1. Scoping

- [ ] The asset list is complete.
- [ ] Prohibited actions are written down.

**Often missed:** cloud is not in the asset list.

#### 2. Reconnaissance

- [ ] External DNS and certificates.

Related: -x id:attack-surface-recon

### The horizontal axis

| Surface | Why it gets skipped | In this guide |
|---|---|---|
| Cloud control plane | Nobody owns it | -x id:cloud-security-fundamentals |

### The ten things

1. **The cloud account**, including who can assume what.
2. **The build pipeline**, which holds everything.

<!-- lang:zh -->
#### 1. 范围

- [ ] 资产清单是完整的。

**常被漏掉：** 云不在资产清单里。

相关：-x id:cloud-security-fundamentals
`

// loadEmbeddedLibrary loads the real knowledge base, so the parser is exercised
// against the entry it actually has to draw and not only against a sample.
func loadEmbeddedLibrary(t *testing.T) *Library {
	t.Helper()
	lib, err := Load(content.FS, content.Root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return lib
}

func parseSample(t *testing.T) *TacticalMap {
	t.Helper()
	tech, err := Parse("offensive/demo/demo-map.md", []byte(sampleMapBody))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	m := TacticalMapOf(tech, LangEN)
	if m == nil {
		t.Fatal("TacticalMapOf returned nil for a checklist body")
	}
	return m
}

func TestTacticalMapParsesStages(t *testing.T) {
	m := parseSample(t)

	if len(m.Stages) != 2 {
		t.Fatalf("stages = %d, want 2", len(m.Stages))
	}
	first := m.Stages[0]
	if first.Number != 1 || first.Title != "Scoping" {
		t.Errorf("stage 1 = %d %q", first.Number, first.Title)
	}
	if len(first.Items) != 2 {
		t.Errorf("stage 1 has %d items, want 2: %v", len(first.Items), first.Items)
	}
	if !strings.Contains(first.Missed, "cloud is not in the asset list") {
		t.Errorf("stage 1 missed = %q", first.Missed)
	}
	if len(first.Related) != 0 {
		t.Errorf("stage 1 related = %v, want none", first.Related)
	}

	second := m.Stages[1]
	if len(second.Items) != 1 {
		t.Errorf("stage 2 has %d items, want 1", len(second.Items))
	}
	if len(second.Related) != 1 || second.Related[0] != "attack-surface-recon" {
		t.Errorf("stage 2 related = %v, want [attack-surface-recon]", second.Related)
	}
	if second.Missed != "" {
		t.Errorf("stage 2 missed = %q, want empty", second.Missed)
	}
}

func TestTacticalMapParsesSurfacesAndTopMissed(t *testing.T) {
	m := parseSample(t)

	if len(m.Surfaces) != 1 {
		t.Fatalf("surfaces = %d, want 1 (the header and rule rows must be skipped)", len(m.Surfaces))
	}
	s := m.Surfaces[0]
	if s.Name != "Cloud control plane" {
		t.Errorf("surface name = %q", s.Name)
	}
	if s.Why != "Nobody owns it" {
		t.Errorf("surface why = %q", s.Why)
	}
	if len(s.Related) != 1 || s.Related[0] != "cloud-security-fundamentals" {
		t.Errorf("surface related = %v", s.Related)
	}

	if len(m.TopMiss) != 2 {
		t.Fatalf("topMissed = %d, want 2: %v", len(m.TopMiss), m.TopMiss)
	}
	if !strings.HasPrefix(m.TopMiss[0], "The cloud account") {
		t.Errorf("topMissed[0] = %q", m.TopMiss[0])
	}
	if !strings.Contains(m.TopMiss[1], "build pipeline") {
		t.Errorf("topMissed[1] = %q", m.TopMiss[1])
	}
}

func TestTacticalMapParsesChinese(t *testing.T) {
	tech, err := Parse("offensive/demo/demo-map.md", []byte(sampleMapBody))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	m := TacticalMapOf(tech, LangZH)
	if m == nil {
		t.Fatal("TacticalMapOf returned nil for the Chinese half")
	}
	if len(m.Stages) != 1 || m.Stages[0].Title != "范围" {
		t.Fatalf("Chinese stages = %+v", m.Stages)
	}
	if !strings.Contains(m.Stages[0].Missed, "云不在资产清单里") {
		t.Errorf("Chinese missed = %q", m.Stages[0].Missed)
	}
	if len(m.Stages[0].Related) != 1 || m.Stages[0].Related[0] != "cloud-security-fundamentals" {
		t.Errorf("Chinese related = %v", m.Stages[0].Related)
	}
}

// A normal technique entry must not be mistaken for a map.
func TestTacticalMapIgnoresOrdinaryEntries(t *testing.T) {
	body := `---
id: ordinary
title_en: Ordinary
title_zh: 普通
---

<!-- lang:en -->
### Why it works

- **A bullet**, but not a checklist item.

#### Not a numbered stage
`
	tech, err := Parse("offensive/demo/ordinary.md", []byte(body))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if m := TacticalMapOf(tech, LangEN); m != nil {
		t.Errorf("TacticalMapOf parsed an ordinary entry into %+v", m)
	}
}

// The real entry must parse: the parser and the content are allowed to break
// each other loudly rather than silently produce a half-drawn map.
func TestTacticalMapParsesTheRealEntry(t *testing.T) {
	lib := loadEmbeddedLibrary(t)
	tech, ok := lib.Get(DefaultMapID)
	if !ok {
		t.Fatalf("%s is not in the library", DefaultMapID)
	}
	for _, lang := range []string{LangEN, LangZH} {
		m := TacticalMapOf(tech, lang)
		if m == nil {
			t.Fatalf("[%s] TacticalMapOf returned nil", lang)
		}
		if len(m.Stages) < 10 {
			t.Errorf("[%s] only %d stages parsed", lang, len(m.Stages))
		}
		if len(m.Surfaces) < 5 {
			t.Errorf("[%s] only %d surfaces parsed", lang, len(m.Surfaces))
		}
		if len(m.TopMiss) < 5 {
			t.Errorf("[%s] only %d top-missed items parsed", lang, len(m.TopMiss))
		}
		for _, st := range m.Stages {
			if len(st.Items) == 0 {
				t.Errorf("[%s] stage %d (%s) has no checklist items", lang, st.Number, st.Title)
			}
		}
		var items, missed, related int
		for _, st := range m.Stages {
			items += len(st.Items)
			if st.Missed != "" {
				missed++
			}
			related += len(st.Related)
		}
		t.Logf("[%s] %d stages, %d items, %d with an often-missed note, %d related links, %d surfaces, %d top-missed",
			lang, len(m.Stages), items, missed, related, len(m.Surfaces), len(m.TopMiss))
	}
}
