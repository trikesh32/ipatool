package http

type ResponseFormat string

const (
	ResponseFormatJSON ResponseFormat = "json"
	ResponseFormatRaw  ResponseFormat = "raw"
	ResponseFormatXML  ResponseFormat = "xml"
)
