package upload

type ClassifiedError struct {
	class   string
	message string
}

func NewClassifiedError(class string, message string) ClassifiedError {
	return ClassifiedError{class: class, message: message}
}

func (e ClassifiedError) Error() string {
	return e.message
}

func (e ClassifiedError) ErrorClass() string {
	return e.class
}
