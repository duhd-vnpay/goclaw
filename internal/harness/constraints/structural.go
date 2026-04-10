package constraints

type StructuralTest interface {
	Name() string
	Applies(toolName string, output string) bool
	Validate(output string) (pass bool, feedback string)
}
