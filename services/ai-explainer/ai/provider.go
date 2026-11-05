package ai

import "context"

// ExplainerProvider streams explanation tokens for learner-facing failures.
// Each provider must preserve the streaming contract used by the server.
type ExplainerProvider interface {
	ExplainCrashStream(
		ctx context.Context,
		command, stderr string,
		exitCode int,
		onToken func(token string, done bool, fallback bool),
	) error

	ExplainSimplerStream(
		ctx context.Context,
		command, stderr string,
		exitCode int,
		onToken func(token string, done bool, fallback bool),
	) error

	ExplainQuestionStream(
		ctx context.Context,
		command, stderr string,
		exitCode int,
		question string,
		onToken func(token string, done bool, fallback bool),
	) error

	// FallbackForCommand is a prepared explanation used when the model is
	// unavailable or the session's budget is spent.
	FallbackForCommand(command string, exitCode int) string
}
