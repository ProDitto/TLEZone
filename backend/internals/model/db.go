package model

type UserDB struct {
	ID       int
	Username string
	Email    string
	Role     string
	Password string
}

type TagDB struct {
	ID   int
	Name string
}

type DifficultyDB struct {
	ID   int
	Name string
}

type ProblemExampleDB struct {
	ID          int
	ProblemID   int
	Input       string
	Expected    string
	Explanation string
}

type ProblemDB struct {
	ID            int
	Title         string
	DifficultyID  int
	Description   string
	Constraints   string
	TimeLimitMS   int
	MemoryLimitKB int
	Score         int
}

type ProgrammingLanguageDB struct {
	ID   int
	Name string
}

type ProblemTagsDB struct {
	ProblemID int
	TagID     int
}

type ProblemReferenceSolutionDB struct {
	ProblemID  int
	LanguageID int
	Code       string
	Status     string
}

type TestCaseDB struct {
	ID             int
	ProblemID      int
	Input          string
	ExpectedOutput string
}

type SubmissionDB struct {
	ID         int
	UserID     int
	ContestID  *int // nil if no contest
	ProblemID  int
	Code       string
	LanguageID int
	Status     string
	RuntimeMS  int
	MemoryKB   int
}
                                              