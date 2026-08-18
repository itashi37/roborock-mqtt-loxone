package templates

type SampleKind string

const (
	VirtualInputDigital SampleKind = "virtual_input_digital"
	VirtualInputAnalog  SampleKind = "virtual_input_analog"
	VirtualTextInput    SampleKind = "virtual_text_input"
	VirtualOutputHTTP   SampleKind = "virtual_output_http"
	VirtualOutputPOST   SampleKind = "virtual_output_http_post"
)

type Status struct {
	NativeGeneration bool         `json:"native_generation"`
	FormatVerified   bool         `json:"format_verified"`
	Reason           string       `json:"reason"`
	RequiredSamples  []SampleKind `json:"required_samples"`
}

// StatusForCurrentBuild deliberately fails closed. A later implementation may
// replace individual requirements with versioned, fixture-backed validators,
// but native XML generation must never be enabled by inference.
func StatusForCurrentBuild() Status {
	return Status{
		NativeGeneration: false,
		FormatVerified:   false,
		Reason:           "native Loxone Config XML generation is locked until real templates exported by the target Loxone Config version are validated",
		RequiredSamples:  []SampleKind{VirtualInputDigital, VirtualInputAnalog, VirtualTextInput, VirtualOutputHTTP, VirtualOutputPOST},
	}
}
