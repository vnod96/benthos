package query

import (
	"fmt"
	"strconv"

	"github.com/Jeffail/benthos/v3/lib/expression/x/parser"
)

//------------------------------------------------------------------------------

type arithmeticOp int

const (
	arithmeticAdd arithmeticOp = iota
	arithmeticSub
	arithmeticDiv
	arithmeticMul
	arithmeticEq
	arithmeticNeq
	arithmeticGt
	arithmeticLt
	arithmeticGte
	arithmeticLte
)

func arithmeticOpParser() parser.Type {
	opParser := parser.AnyOf(
		parser.Char('+'),
		parser.Char('-'),
		parser.Char('/'),
		parser.Char('*'),
		parser.Match("=="),
		parser.Match("!="),
		parser.Match(">="),
		parser.Match("<="),
		parser.Char('>'),
		parser.Char('<'),
	)
	return func(input []rune) parser.Result {
		res := opParser(input)
		if res.Err != nil {
			return res
		}
		switch res.Result.(string) {
		case "+":
			res.Result = arithmeticAdd
		case "-":
			res.Result = arithmeticSub
		case "/":
			res.Result = arithmeticDiv
		case "*":
			res.Result = arithmeticMul
		case "==":
			res.Result = arithmeticEq
		case "!=":
			res.Result = arithmeticNeq
		case ">":
			res.Result = arithmeticGt
		case "<":
			res.Result = arithmeticLt
		case ">=":
			res.Result = arithmeticGte
		case "<=":
			res.Result = arithmeticLte
		default:
			return parser.Result{
				Remaining: input,
				Err:       fmt.Errorf("operator not recognized: %v", res.Result),
			}
		}
		return res
	}
}

func getNumber(v interface{}) (float64, error) {
	switch t := v.(type) {
	case int64:
		return float64(t), nil
	case float64:
		return t, nil
	case string:
		return strconv.ParseFloat(t, 64)
	}
	return 0, fmt.Errorf("function returned non-numerical type: %T", v)
}

func add(fns []Function) Function {
	return closureFn(func(i int, msg Message, legacy bool) (interface{}, error) {
		var total float64
		var err error

		for _, fn := range fns {
			var nextF float64
			next, tmpErr := fn.Exec(i, msg, legacy)
			if tmpErr == nil {
				nextF, tmpErr = getNumber(next)
			}
			if tmpErr != nil {
				err = tmpErr
				continue
			}
			total += nextF
		}

		if err != nil {
			return nil, &ErrRecoverable{
				Err:       err,
				Recovered: total,
			}
		}
		return total, nil
	})
}

func sub(lhs, rhs Function) Function {
	return closureFn(func(i int, msg Message, legacy bool) (interface{}, error) {
		var total float64
		var err error

		if leftV, tmpErr := lhs.Exec(i, msg, legacy); tmpErr == nil {
			total, err = getNumber(leftV)
		} else {
			err = tmpErr
		}
		if rightV, tmpErr := rhs.Exec(i, msg, legacy); tmpErr == nil {
			var toSub float64
			if toSub, tmpErr = getNumber(rightV); tmpErr != nil {
				err = tmpErr
			} else {
				total -= toSub
			}
		} else {
			err = tmpErr
		}

		if err != nil {
			return nil, &ErrRecoverable{
				Err:       err,
				Recovered: total,
			}
		}
		return total, nil
	})
}

func divide(lhs, rhs Function) Function {
	return closureFn(func(i int, msg Message, legacy bool) (interface{}, error) {
		var result float64
		var err error

		if leftV, tmpErr := lhs.Exec(i, msg, legacy); tmpErr == nil {
			result, err = getNumber(leftV)
		} else {
			err = tmpErr
		}
		if rightV, tmpErr := rhs.Exec(i, msg, legacy); tmpErr == nil {
			var denom float64
			if denom, tmpErr = getNumber(rightV); tmpErr != nil {
				err = tmpErr
			} else {
				result = result / denom
			}
		} else {
			err = tmpErr
		}

		if err != nil {
			return nil, err
		}
		return result, nil
	})
}

func multiply(lhs, rhs Function) Function {
	return closureFn(func(i int, msg Message, legacy bool) (interface{}, error) {
		var result float64
		var err error

		if leftV, tmpErr := lhs.Exec(i, msg, legacy); tmpErr == nil {
			result, err = getNumber(leftV)
		} else {
			err = tmpErr
		}
		if rightV, tmpErr := rhs.Exec(i, msg, legacy); tmpErr == nil {
			var denom float64
			if denom, tmpErr = getNumber(rightV); tmpErr != nil {
				err = tmpErr
			} else {
				result = result * denom
			}
		} else {
			err = tmpErr
		}

		if err != nil {
			return nil, err
		}
		return result, nil
	})
}

func compare(lhs, rhs Function, op arithmeticOp) (Function, error) {
	var opFn func(lhs, rhs float64) bool
	switch op {
	case arithmeticEq:
		opFn = func(lhs, rhs float64) bool {
			return lhs == rhs
		}
	case arithmeticNeq:
		opFn = func(lhs, rhs float64) bool {
			return lhs != rhs
		}
	case arithmeticGt:
		opFn = func(lhs, rhs float64) bool {
			return lhs > rhs
		}
	case arithmeticGte:
		opFn = func(lhs, rhs float64) bool {
			return lhs >= rhs
		}
	case arithmeticLt:
		opFn = func(lhs, rhs float64) bool {
			return lhs < rhs
		}
	case arithmeticLte:
		opFn = func(lhs, rhs float64) bool {
			return lhs <= rhs
		}
	default:
		return nil, fmt.Errorf("operator not supported: %v", op)
	}
	return closureFn(func(i int, msg Message, legacy bool) (interface{}, error) {
		var lhsV, rhsV float64
		var err error

		if leftV, tmpErr := lhs.Exec(i, msg, legacy); tmpErr == nil {
			lhsV, err = getNumber(leftV)
		} else {
			err = tmpErr
		}
		if rightV, tmpErr := rhs.Exec(i, msg, legacy); tmpErr == nil {
			if rhsV, tmpErr = getNumber(rightV); tmpErr != nil {
				err = tmpErr
			}
		} else {
			err = tmpErr
		}
		if err != nil {
			return nil, err
		}
		return opFn(lhsV, rhsV), nil
	}), nil
}

func resolveArithmetic(fns []Function, ops []arithmeticOp) (Function, error) {
	if len(fns) == 1 && len(ops) == 0 {
		return fns[0], nil
	}
	if len(fns) != (len(ops) + 1) {
		return nil, fmt.Errorf("mismatch of functions to arithmetic operators")
	}

	// First pass to resolve division and multiplication
	fnsNew, opsNew := []Function{fns[0]}, []arithmeticOp{}
	for i, op := range ops {
		switch op {
		case arithmeticMul:
			fnsNew[len(fnsNew)-1] = multiply(fnsNew[len(fnsNew)-1], fns[i+1])
		case arithmeticDiv:
			fnsNew[len(fnsNew)-1] = divide(fnsNew[len(fnsNew)-1], fns[i+1])
		default:
			fnsNew = append(fnsNew, fns[i+1])
			opsNew = append(opsNew, op)
		}
	}
	fns, ops = fnsNew, opsNew
	if len(fns) == 1 {
		return fns[0], nil
	}

	// Next, resolve additions and subtractions
	var addPile, subPile []Function
	addPile = append(addPile, fns[0])
	for i, op := range ops {
		switch op {
		case arithmeticAdd:
			addPile = append(addPile, fns[i+1])
		case arithmeticSub:
			subPile = append(subPile, fns[i+1])
		case arithmeticEq,
			arithmeticNeq,
			arithmeticGt,
			arithmeticGte,
			arithmeticLt,
			arithmeticLte:
			var rhs Function
			lhs, err := resolveArithmetic(fns[:i+1], ops[:i])
			if err == nil {
				rhs, err = resolveArithmetic(fns[i+1:], ops[i+1:])
			}
			if err != nil {
				return nil, err
			}
			return compare(lhs, rhs, op)
		}
	}

	fn := add(addPile)
	if len(subPile) > 0 {
		fn = sub(fn, add(subPile))
	}
	return fn, nil
}

//------------------------------------------------------------------------------