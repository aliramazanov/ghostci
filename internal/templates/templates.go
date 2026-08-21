package templates

import (
	"bytes"
	_ "embed"
	"text/template"
)

//go:embed prepush.sh
var prePush string

type PrePushParams struct {
	Binary string
}

func PrePush(p PrePushParams) (string, error) {
	tmpl, err := template.New("pre-push").Parse(prePush)
	if err != nil {
		return "", err
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, p); err != nil {
		return "", err
	}

	return out.String(), nil
}
