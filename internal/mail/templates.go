package mail

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	texttemplate "text/template"
)

type messageBody struct {
	Text string
	HTML string
}

var (
	docTextTmpl = texttemplate.Must(texttemplate.New("docText").Parse(docTextSrc))
	docHTMLTmpl = htmltemplate.Must(htmltemplate.New("docHTML").Parse(docHTMLSrc))
	pwTextTmpl  = texttemplate.Must(texttemplate.New("pwText").Parse(pwTextSrc))
	pwHTMLTmpl  = htmltemplate.Must(htmltemplate.New("pwHTML").Parse(pwHTMLSrc))
)

func renderDocumentBody(orgName string) (messageBody, error) {
	return render(docTextTmpl, docHTMLTmpl, map[string]any{"OrgName": orgName})
}

func renderPasswordBody(password string) (messageBody, error) {
	return render(pwTextTmpl, pwHTMLTmpl, map[string]any{"Password": password})
}

func render(text *texttemplate.Template, html *htmltemplate.Template, data any) (messageBody, error) {
	var tb bytes.Buffer
	if err := text.Execute(&tb, data); err != nil {
		return messageBody{}, fmt.Errorf("renderowanie treści tekstowej: %w", err)
	}

	var hb bytes.Buffer
	if err := html.Execute(&hb, data); err != nil {
		return messageBody{}, fmt.Errorf("renderowanie treści HTML: %w", err)
	}

	return messageBody{Text: tb.String(), HTML: hb.String()}, nil
}

const docTextSrc = `Dzień dobry,

w załączeniu przesyłamy dokument w formie zaszyfrowanego pliku PDF.
Hasło do jego otwarcia zostanie przesłane w osobnej wiadomości.

Pozdrawiamy,
{{.OrgName}}
`

const docHTMLSrc = `<!DOCTYPE html>
<html lang="pl"><body style="font-family:sans-serif;color:#1f2328;">
<p>Dzień dobry,</p>
<p>w załączeniu przesyłamy dokument w formie zaszyfrowanego pliku PDF.
Hasło do jego otwarcia zostanie przesłane w osobnej wiadomości.</p>
<p>Pozdrawiamy,<br>{{.OrgName}}</p>
</body></html>
`

const pwTextSrc = `Dzień dobry,

kod dostępu do wcześniej przesłanego dokumentu:

    {{.Password}}

Prosimy nie przekazywać go dalej.
`

const pwHTMLSrc = `<!DOCTYPE html>
<html lang="pl"><body style="font-family:sans-serif;color:#1f2328;">
<p>Dzień dobry,</p>
<p>kod dostępu do wcześniej przesłanego dokumentu:</p>
<p style="font-size:1.2rem;font-weight:bold;letter-spacing:0.05em;">{{.Password}}</p>
<p>Prosimy nie przekazywać go dalej.</p>
</body></html>
`
