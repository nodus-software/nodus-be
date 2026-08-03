package clinical

import "context"

type Repository interface {
	ListResources(context.Context, string) ([]Resource, error)
	CreateResource(context.Context, string, Resource) (*Resource, error)
	CreateVisit(context.Context, Visit) (*Visit, error)
	GetVisit(context.Context, string) (*Visit, error)
	ApplyVisitRouting(context.Context, Visit) error
	ListQueueEntries(context.Context, string) ([]QueueEntry, error)
	GetQueueEntry(context.Context, string) (*QueueEntry, error)
	CreateQueueEntry(context.Context, QueueEntry, *string, bool) (*QueueEntry, error)
	TransitionQueueEntry(context.Context, QueueEntry, string, string, *string, *string, bool) (*QueueEntry, error)
	ListQueueHistory(context.Context, string) ([]QueueHistory, error)
	ListRoutingRules(context.Context) ([]RoutingRule, error)
	CreateRoutingRule(context.Context, RoutingRule) (*RoutingRule, error)
	SearchConcepts(context.Context, string, int) ([]Concept, error)
	FindActiveOutpatientVisit(context.Context, string) (*Visit, error)
	ListVisits(context.Context, string, string) ([]Visit, error)
	CreateEncounter(context.Context, Encounter) (*Encounter, error)
	GetEncounter(context.Context, string) (*Encounter, error)
	ListEncounters(context.Context, string) ([]Encounter, error)
	CompleteEncounter(context.Context, string, string, *string) (*Encounter, error)
	CreateObservations(context.Context, []Observation) ([]Observation, error)
	ListObservations(context.Context, string) ([]Observation, error)
	CreateNote(context.Context, ClinicalNote) (*ClinicalNote, error)
	ListNotes(context.Context, string) ([]ClinicalNote, error)
	CreateDiagnosis(context.Context, Diagnosis) (*Diagnosis, error)
	ListDiagnoses(context.Context, string) ([]Diagnosis, error)
	CreateAllergy(context.Context, Allergy) (*Allergy, error)
	ListAllergies(context.Context, string) ([]Allergy, error)
	CompleteVisit(context.Context, string) (*Visit, error)
}
