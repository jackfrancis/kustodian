package basicserver

import (
	"net/http"
	"os"
	"testing"

	clarkezoneLog "github.com/jackfrancis/kustodian/pkg/log"
	"github.com/sirupsen/logrus"
)

// TestMain initizlie all tests
func TestMain(m *testing.M) {
	clarkezoneLog.Init(logrus.DebugLevel)
	code := m.Run()
	os.Exit(code)
}

func Test_webhooklistening(t *testing.T) {
	wh := BasicServer{}
	mux := http.NewServeMux()
	wh.StartListen(mux)
	// TODO: how to wait for server async start
	err := wh.Shutdown()
	if err != nil {
		t.Errorf("shutdown failed")
	}
}
