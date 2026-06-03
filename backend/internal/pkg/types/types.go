package types

type Expectation struct {
	URLContains    string `json:"urlContains,omitempty"`
	TextVisible    string `json:"textVisible,omitempty"`
	ElementVisible string `json:"elementVisible,omitempty"`
	ElementCount   *struct {
		Selector string `json:"selector"`
		Count    int    `json:"count"`
	} `json:"elementCount,omitempty"`
	ValueEquals *struct {
		Selector string `json:"selector"`
		Value    string `json:"value"`
	} `json:"valueEquals,omitempty"`
}

type RecoveryHint struct {
	Intent              string `json:"intent,omitempty"`
	ExpectedURLContains string `json:"expectedUrlContains,omitempty"`
	ExpectedText        string `json:"expectedText,omitempty"`
}

type Step struct {
	ID            string        `json:"id"`
	Description   string        `json:"description,omitempty"`
	Action        string        `json:"action"`
	Target        string        `json:"target,omitempty"`
	Selector      string        `json:"selector,omitempty"`
	Value         string        `json:"value,omitempty"`
	Key           string        `json:"key,omitempty"`
	Expect        *Expectation  `json:"expect,omitempty"`
	AllowRecovery bool          `json:"allowRecovery,omitempty"`
	Recovery      *RecoveryHint `json:"recovery,omitempty"`
	Ungrounded    bool          `json:"ungrounded,omitempty"`
	GroundingNote string        `json:"groundingNote,omitempty"`
}

type TestSpec struct {
	Name    string `json:"name"`
	BaseURL string `json:"baseUrl"`
	Steps   []Step `json:"steps"`
}

type GenerateRequest struct {
	Intent             string `json:"intent"`
	ForceDriftFallback bool   `json:"forceDriftFallback,omitempty"`
}

type GenerateResponse struct {
	Spec       *TestSpec `json:"spec,omitempty"`
	Validation struct {
		Valid  bool     `json:"valid"`
		Errors []string `json:"errors,omitempty"`
	} `json:"validation"`
	UsedFallback      bool     `json:"usedFallback"`
	GroundingWarnings []string `json:"groundingWarnings,omitempty"`
}

type RunRequest struct {
	Spec TestSpec `json:"spec"`
}

type RunResult struct {
	RunID               string               `json:"runId"`
	Status              string               `json:"status"`
	StartedAt           string               `json:"startedAt"`
	FinishedAt          string               `json:"finishedAt"`
	Steps               []StepResult         `json:"steps"`
	PromotionCandidates []PromotionCandidate `json:"promotionCandidates,omitempty"`
	Artifacts           Artifacts            `json:"artifacts"`
	Error               string               `json:"error,omitempty"`
}

type StepResult struct {
	StepID             string              `json:"stepId"`
	Action             string              `json:"action"`
	Mode               string              `json:"mode,omitempty"`
	Status             string              `json:"status"`
	DurationMs         int64               `json:"durationMs"`
	Message            string              `json:"message,omitempty"`
	UnverifiedRecovery bool                `json:"unverifiedRecovery,omitempty"`
	Recovery           *RecoveryResult     `json:"recovery,omitempty"`
	DecisionTrace      *DecisionTrace      `json:"decisionTrace,omitempty"`
	PromotionCandidate *PromotionCandidate `json:"promotionCandidate,omitempty"`
}

type RecoveryResult struct {
	Intent            string `json:"intent,omitempty"`
	FailedSelector    string `json:"failedSelector,omitempty"`
	RecoveredSelector string `json:"recoveredSelector,omitempty"`
	Strategy          string `json:"strategy,omitempty"`
	CandidateCount    int    `json:"candidateCount,omitempty"`
	ScreenshotPath    string `json:"screenshotPath,omitempty"`
	Reasoning         string `json:"reasoning,omitempty"`
	Evidence          string `json:"evidence,omitempty"`
	Message           string `json:"message,omitempty"`
}

type DecisionTrace struct {
	Mode                string `json:"mode,omitempty"`
	AttemptedAction     string `json:"attemptedAction,omitempty"`
	AttemptedSelector   string `json:"attemptedSelector,omitempty"`
	Failure             string `json:"failure,omitempty"`
	AgentDecision       string `json:"agentDecision,omitempty"`
	SelectedSelector    string `json:"selectedSelector,omitempty"`
	Reasoning           string `json:"reasoning,omitempty"`
	CustomerExplanation string `json:"customerExplanation,omitempty"`
	Evidence            string `json:"evidence,omitempty"`
}

type PromotionCandidate struct {
	ID                  string `json:"id"`
	RunID               string `json:"runId,omitempty"`
	StepID              string `json:"stepId"`
	Status              string `json:"status"`
	OriginalAction      string `json:"originalAction,omitempty"`
	OriginalSelector    string `json:"originalSelector,omitempty"`
	ProposedAction      string `json:"proposedAction"`
	ProposedSelector    string `json:"proposedSelector"`
	Reasoning           string `json:"reasoning,omitempty"`
	CustomerExplanation string `json:"customerExplanation,omitempty"`
	Evidence            string `json:"evidence,omitempty"`
	ReviewReason        string `json:"reviewReason,omitempty"`
	PromotedSpecPath    string `json:"promotedSpecPath,omitempty"`
}

type PromotionRequest struct {
	RunID  string `json:"runId"`
	Reason string `json:"reason,omitempty"`
}

type PromotionResponse struct {
	Promotion   PromotionCandidate `json:"promotion"`
	UpdatedSpec *TestSpec          `json:"updatedSpec,omitempty"`
	RunResult   *RunResult         `json:"runResult,omitempty"`
}

type Artifacts struct {
	ReportPath        string `json:"reportPath,omitempty"`
	FailureScreenshot string `json:"failureScreenshot,omitempty"`
}
