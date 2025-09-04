package handlers

import "encoding/json"

type JsonMarshaller interface {
	MarshalIndent(v any, prefix, indent string) ([]byte, error)
}

func NewJsonMarshaller() JsonMarshaller {
	return &DefaultJsonMarshaller{}
}

type DefaultJsonMarshaller struct {
}

func (j *DefaultJsonMarshaller) MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	return json.MarshalIndent(v, prefix, indent)
}
