package pipeline

import "fmt"

type UndecidableError struct{ Context string }

func (e *UndecidableError) Error() string {
	return fmt.Sprintf("depends on %s, which is only known inside CI", e.Context)
}

type Value struct {
	Text string

	Defined bool

	Unknown bool

	Source string
}

func Known(text string) Value { return Value{Text: text, Defined: true} }

func Undefined() Value { return Value{} }

func Unknown(source string) Value { return Value{Unknown: true, Source: source} }

func (v Value) Truthy() bool { return v.Defined && v.Text != "" }

func (v Value) Err() error {
	if !v.Unknown {
		return nil
	}

	source := v.Source
	if source == "" {
		source = "state only CI has"
	}

	return &UndecidableError{Context: source}
}
