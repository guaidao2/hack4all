package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/guaidao2/hack4all/internal/core"
)

// The page draws the coverage map from the same Markdown file the CLI prints, so
// the API has to serve the parsed form in both languages.
func TestAPITacticalMapServesTheParsedMap(t *testing.T) {
	api, _ := testAPI(t)

	for _, lang := range []string{core.LangEN, core.LangZH} {
		rec := get(t, api, "/api/tactical-map?lang="+lang)
		if rec.Code != http.StatusOK {
			t.Fatalf("[%s] status %d: %s", lang, rec.Code, rec.Body.String())
		}
		var m core.TacticalMap
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
			t.Fatalf("[%s] decode: %v", lang, err)
		}
		if len(m.Stages) < 10 {
			t.Errorf("[%s] only %d stages", lang, len(m.Stages))
		}
		if len(m.Surfaces) == 0 {
			t.Errorf("[%s] no surfaces parsed", lang)
		}
		if len(m.TopMiss) == 0 {
			t.Errorf("[%s] no top-missed items parsed", lang)
		}
		for _, st := range m.Stages {
			if st.Title == "" || len(st.Items) == 0 {
				t.Errorf("[%s] stage %d is empty: %+v", lang, st.Number, st)
			}
		}
	}
}

// A bare fetch has to work: that is what the page does on load.
func TestAPITacticalMapDefaultsToTheRedTeamMap(t *testing.T) {
	api, _ := testAPI(t)

	rec := get(t, api, "/api/tactical-map")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var m core.TacticalMap
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if strings.TrimSpace(m.Title) == "" {
		t.Error("the default map has no title")
	}
}

func TestAPITacticalMapRejectsNonMaps(t *testing.T) {
	api, _ := testAPI(t)

	rec := get(t, api, "/api/tactical-map?id=nope")
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown id: status %d, want 404", rec.Code)
	}

	// An ordinary technique is not a coverage map. Saying so is better than
	// returning an empty map, which the page would draw as a blank sheet.
	rec = get(t, api, "/api/tactical-map?id=kerberoasting")
	if rec.Code != http.StatusNotFound {
		t.Errorf("non-map entry: status %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "not a coverage map") {
		t.Errorf("error body = %s", rec.Body.String())
	}
}
