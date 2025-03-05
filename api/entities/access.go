package entities

type AccessEntity interface {
	// TypeIdentifier is a lower-cased type identifier for the AccessEntity. It is used as a prefix for namespace-scoped resources such as role bindings.
	TypeIdentifier() string

	// GetName returns the name of the AccessEntity instance.
	GetName() string
}

type AccessRole interface {
	// EntityType references the AccessEntity that this role belongs to.
	EntityType() AccessEntity

	// Identifier is a lower-cased type identifier for this role, scoped to the AccessEntity returned by EntityType().
	Identifier() string
}
