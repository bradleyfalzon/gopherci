package logger

import (
	"bytes"
	"regexp"
	"testing"
)

func TestLogger(t *testing.T) {

	wantDevelopment := `time="" level=debug msg=debugarg logger=gci server_name= 
time="" level=debug msg="debugf arg" logger=gci server_name= 
time="" level=info msg=infoarg logger=gci server_name= 
time="" level=info msg="infof arg" logger=gci server_name= 
time="" level=error msg=errorarg logger=gci server_name= 
time="" level=error msg="errorf arg" logger=gci server_name= 
time="" level=info msg=context key=value logger=gci server_name= 
`

	wantProduction := `{"level":"info","logger":"gci","msg":"infoarg","server_name":"","time":""}
{"level":"info","logger":"gci","msg":"infof arg","server_name":"","time":""}
{"level":"error","logger":"gci","msg":"errorarg","server_name":"","time":""}
{"level":"error","logger":"gci","msg":"errorf arg","server_name":"","time":""}
{"key":"value","level":"info","logger":"gci","msg":"context","server_name":"","time":""}
`

	tests := map[string]struct {
		env  string
		want string
	}{
		"development": {env: "development", want: wantDevelopment},
		"production":  {env: "production", want: wantProduction},
	}

	for desc, test := range tests {
		var out bytes.Buffer

		l := New(&out, "buildabc", test.env, "")

		l.Debug("debug", "arg")
		l.Debugf("debugf %s", "arg")

		l.Info("info", "arg")
		l.Infof("infof %s", "arg")

		l.Error("error", "arg")
		l.Errorf("errorf %s", "arg")

		l.With("key", "value").Info("context")

		have := out.String()
		have = regexp.MustCompile(`time="[^"]+"`).ReplaceAllString(have, `time=""`)
		have = regexp.MustCompile(`"time":"[^"]+"`).ReplaceAllString(have, `"time":""`)
		have = regexp.MustCompile(`server_name=[a-zA-Z0-9.-]+`).ReplaceAllString(have, `server_name=`)
		have = regexp.MustCompile(`"server_name":"[^"]+"`).ReplaceAllString(have, `"server_name":""`)

		if have != test.want {
			t.Errorf("desc: %s:\nhave:\n%swant:\n%s", desc, have, test.want)
		}
	}
}
