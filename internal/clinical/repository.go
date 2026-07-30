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
}
