package model

type Severity string

const (
	SeverityCritical Severity = "CRITICAL"
	SeverityHigh     Severity = "HIGH"
	SeverityMedium   Severity = "MEDIUM"
	SeverityLow      Severity = "LOW"
	SeverityInfo     Severity = "INFO"
)

type Layer struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

type Recipe struct {
	Path      string            `json:"path"`
	Layer     string            `json:"layer"`
	Name      string            `json:"name"`
	PN        string            `json:"pn"`
	PV        string            `json:"pv"`
	Variables map[string]string `json:"variables"`
	Lines     []string          `json:"-"`
}

type Append struct {
	Path      string            `json:"path"`
	Layer     string            `json:"layer"`
	Name      string            `json:"name"`
	Target    string            `json:"target"`
	Variables map[string]string `json:"variables"`
	Lines     []string          `json:"-"`
}

type Patch struct {
	Path  string   `json:"path"`
	Layer string   `json:"layer"`
	Lines []string `json:"-"`
}

type Finding struct {
	RuleID       string   `json:"rule_id"`
	Title        string   `json:"title"`
	Severity     Severity `json:"severity"`
	Layer        string   `json:"layer"`
	File         string   `json:"file"`
	Line         int      `json:"line"`
	Message      string   `json:"message"`
	WhyItMatters string   `json:"why_it_matters"`
	Remediation  string   `json:"remediation"`
}

type Report struct {
	Root     string    `json:"root"`
	Layers   []Layer   `json:"layers"`
	Recipes  []Recipe  `json:"recipes"`
	Appends  []Append  `json:"appends"`
	Patches  []Patch   `json:"patches"`
	Findings []Finding `json:"findings"`
}

type ScanPhase string

const (
	PhaseStarting    ScanPhase = "starting"
	PhaseDiscovering ScanPhase = "discovering"
	PhaseParsing     ScanPhase = "parsing"
	PhaseRules       ScanPhase = "rules"
	PhaseDone        ScanPhase = "done"
)

type ScanProgress struct {
	Phase          ScanPhase `json:"phase"`
	CurrentPath    string    `json:"current_path"`
	LayersFound    int       `json:"layers_found"`
	RecipesFound   int       `json:"recipes_found"`
	AppendsFound   int       `json:"appends_found"`
	PatchesFound   int       `json:"patches_found"`
	FilesProcessed int       `json:"files_processed"`
	FindingsFound  int       `json:"findings_found"`
	Done           bool      `json:"done"`
	Error          string    `json:"error"`
}
