package logger

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestRedactsSensitiveKeys(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, Options{Level: slog.LevelInfo, IsProduction: true})
	log.Info("x", "db_password", "p1", "access_token", "t1", "Authorization", "a1", slog.Group("req", "cookie", "c1"), "user", "ok")

	out := buf.String()
	for _, leaked := range []string{"p1", "t1", "a1", "c1"} {
		if strings.Contains(out, `"`+leaked+`"`) {
			t.Fatalf("%s leaked: %s", leaked, out)
		}
	}
	if !strings.Contains(out, `"user":"ok"`) {
		t.Fatalf("non-sensitive key dropped: %s", out)
	}
}
