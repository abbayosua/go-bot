package agent

import "testing"

func TestGateReview(t *testing.T) {
	cases := []struct {
		name      string
		user      string
		assistant string
		fails     int
		denied    int
		want      bool
	}{
		{"quiet turn", "halo", "halo juga", 0, 0, false},
		{"explicit remember id", "tolong ingat ini ya", "ok", 0, 0, true},
		{"explicit remember en", "please remember this", "ok", 0, 0, true},
		{"style correction", "jawab singkat saja", "maaf kepanjangan", 0, 0, true},
		{"tool failures", "lanjut", "coba lagi", 2, 0, true},
		{"one failure quiet", "lanjut", "ok", 1, 0, false},
		{"denied approval", "lanjut", "ok", 0, 1, true},
		{"correction", "bukan begitu maksud saya", "ok", 0, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := gateReview(tc.user, tc.assistant, tc.fails, tc.denied)
			if got.fired != tc.want {
				t.Fatalf("fired=%v want %v (reason %q)", got.fired, tc.want, got.reason)
			}
		})
	}
}

func TestExtractReviewJSON(t *testing.T) {
	in := "```json\n{\"memory\":\"x\",\"skill\":\"\"}\n```"
	if got := extractReviewJSON(in); got != `{"memory":"x","skill":""}` {
		t.Fatalf("got %q", got)
	}
	if got := extractReviewJSON(`{"memory":"","skill":""}`); got == "" {
		t.Fatalf("empty extract")
	}
}
