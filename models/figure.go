package models

type Figure struct {
	ID                string `json:"id"`
	SubjectID         string `json:"subject_id"`
	Figure            string `json:"figure"`
	OnDay             string `json:"on_day"`
	Value             string `json:"value"`
	FunctionCode      string `json:"function_code"`
	FunctionVersion   string `json:"function_version"`
	InputsFingerprint string `json:"inputs_fingerprint"`
	ActorID           string `json:"actor_id"`
	Narration         string `json:"narration,omitempty"`
	RecordedAt        string `json:"recorded_at"`
}
