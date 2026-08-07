package clinical

func templatePtr[T any](v T) *T { return &v }

var DefaultTriageTemplate = TemplateDefinition{SchemaVersion: 1, Sections: []TemplateSection{
	{Key: "vitals", Title: "Vital signs", Fields: []TemplateField{
		{Key: "temperature", Label: "Temperature", Type: "number", Required: true, Binding: &TemplateFieldBinding{Kind: "observation", Code: "temperature", Unit: templatePtr("Cel")}},
		{Key: "pulse", Label: "Pulse", Type: "number", Required: true, Binding: &TemplateFieldBinding{Kind: "observation", Code: "pulse", Unit: templatePtr("/min")}},
		{Key: "respiratory_rate", Label: "Respiratory rate", Type: "number", Required: true, Binding: &TemplateFieldBinding{Kind: "observation", Code: "respiratory-rate", Unit: templatePtr("/min")}},
		{Key: "systolic_bp", Label: "Systolic blood pressure", Type: "number", Required: true, Binding: &TemplateFieldBinding{Kind: "observation", Code: "blood-pressure-systolic", Unit: templatePtr("mm[Hg]")}},
		{Key: "diastolic_bp", Label: "Diastolic blood pressure", Type: "number", Required: true, Binding: &TemplateFieldBinding{Kind: "observation", Code: "blood-pressure-diastolic", Unit: templatePtr("mm[Hg]")}},
		{Key: "oxygen_saturation", Label: "Oxygen saturation", Type: "number", Validation: &TemplateValidation{Min: templatePtr(0.0), Max: templatePtr(100.0)}, Binding: &TemplateFieldBinding{Kind: "observation", Code: "oxygen-saturation", Unit: templatePtr("%")}},
		{Key: "weight", Label: "Weight", Type: "number", Binding: &TemplateFieldBinding{Kind: "observation", Code: "body-weight", Unit: templatePtr("kg")}},
		{Key: "height", Label: "Height", Type: "number", Binding: &TemplateFieldBinding{Kind: "observation", Code: "body-height", Unit: templatePtr("cm")}},
	}},
	{Key: "notes", Title: "Triage notes", Fields: []TemplateField{{Key: "triage_notes", Label: "Triage notes", Type: "long_text"}}},
}}

var DefaultConsultationTemplate = TemplateDefinition{SchemaVersion: 1, Sections: []TemplateSection{
	{Key: "history", Title: "History", Fields: []TemplateField{
		{Key: "presenting_complaint", Label: "Presenting complaint and history", Type: "long_text", Required: true},
		{Key: "medical_history", Label: "Relevant medical history", Type: "long_text"},
	}},
	{Key: "examination", Title: "Examination", Fields: []TemplateField{{Key: "examination_findings", Label: "Examination findings", Type: "long_text"}}},
	{Key: "assessment_plan", Title: "Assessment and plan", Fields: []TemplateField{
		{Key: "clinical_assessment", Label: "Clinical assessment", Type: "long_text", Required: true},
		{Key: "management_plan", Label: "Management plan", Type: "long_text", Required: true},
	}},
}}
