package platform

import "context"

// OnboardingResults is a group of elements required for first run.
type OnboardingResults struct {
	User   *User          `json:"user"`
	Org    *Organization  `json:"org"`
	Bucket *Bucket        `json:"bucket"`
	Auth   *Authorization `json:"auth"`
}

// OnboardingRequest is the request
// to setup defaults.
type OnboardingRequest struct {
	User     string `json:"username"`
	Password string `json:"password"`
	Org      string `json:"org"`
	Bucket   string `json:"bucket"`
}

// OnboardingService represents a service for the first run.
type OnboardingService interface {
	// IsOnboarding determine if onboarding request is allowed.
	IsOnboarding(ctx context.Context) (bool, error)
	// Generate OnboardingResults.
	Generate(ctx context.Context, req *OnboardingRequest) (*OnboardingResults, error)
}
