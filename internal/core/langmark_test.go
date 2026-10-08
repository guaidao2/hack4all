package core

import (
	"strings"
	"testing"
)

// A duplicated language marker used to be accepted and then silently overwrite
// the section it repeated, because splitLanguages assigns per language rather
// than appending. Losing half an entry without any error is exactly the kind of
// failure this project refuses to tolerate.
func TestParseRejectsDuplicateLanguageMarkers(t *testing.T) {
	for _, lang := range []string{"en", "zh"} {
		body := `---
id: page
title_en: Page
title_zh: 页面
---

<!-- lang:en -->
English section.

<!-- lang:zh -->
中文段落。

<!-- lang:` + lang + ` -->
second ` + lang + ` section
`
		t.Run(lang, func(t *testing.T) {
			_, err := Parse("offensive/test/page.md", []byte(body))
			if err == nil {
				t.Fatalf("a duplicate <!-- lang:%s --> marker was accepted", lang)
			}
			if !strings.Contains(err.Error(), lang) {
				t.Errorf("error should name the duplicated language, got: %v", err)
			}
		})
	}
}

// The normal case must keep working: one marker per language, in either order.
func TestParseAcceptsOneMarkerPerLanguage(t *testing.T) {
	body := `---
id: page
title_en: Page
title_zh: 页面
---

<!-- lang:en -->
English section.

<!-- lang:zh -->
中文段落。
`
	tech, err := Parse("offensive/test/page.md", []byte(body))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !strings.Contains(tech.Body.EN, "English section") {
		t.Errorf("English body = %q", tech.Body.EN)
	}
	if !strings.Contains(tech.Body.ZH, "中文段落") {
		t.Errorf("Chinese body = %q", tech.Body.ZH)
	}
}
