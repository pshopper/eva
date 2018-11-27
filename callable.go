package pool

// Callable is interface, that Call function for task in goroutines pool.
type Callable interface {
	// Call perform function.
	Call()

	// Error returns function Call error.
	Error() error

	// Result returns function Call result.
	Result() interface{}
}

// CustomCallable (implement Callable interface) for task in goroutines pool.
type CustomCallable struct {
	f      func(...interface{}) (interface{}, error)
	args   []interface{}
	result interface{}
	error  error
}

// NewCustomCallable is constructor for CustomCallable
func NewCustomCallable(f func(...interface{}) (interface{}, error), args ...interface{}) *CustomCallable {
	return &CustomCallable{
		f:    f,
		args: args,
	}
}

// Call perform function.
func (cc *CustomCallable) Call() {
	cc.result, cc.error = cc.f(cc.args...)
}

// Error returns function Call error.
func (cc *CustomCallable) Error() error {
	return cc.error
}

// Result returns function Call result.
func (cc *CustomCallable) Result() interface{} {
	return cc.result
}
