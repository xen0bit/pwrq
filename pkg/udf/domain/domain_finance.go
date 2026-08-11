package domain

import (
	"fmt"
	"math"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// numArg reads the nth argument as a number.
func numArg(args []any, i int, name string) (float64, error) {
	f, ok := common.ToFloat64(common.BindValue(args[i]))
	if !ok {
		return 0, fmt.Errorf("%s: argument %d is not a number (%T)", name, i+1, args[i])
	}
	return f, nil
}

// RegisterCAGR registers cagr, the compound annual growth rate from a starting
// value to an ending value over a number of years.
func RegisterCAGR() gojq.CompilerOption {
	return gojq.WithFunction("cagr", 3, 3, func(v any, args []any) any {
		start, err1 := numArg(args, 0, "cagr")
		end, err2 := numArg(args, 1, "cagr")
		years, err3 := numArg(args, 2, "cagr")
		if err1 != nil || err2 != nil || err3 != nil {
			return common.MakeUDFErrorResult(firstErr(err1, err2, err3), nil)
		}
		if start <= 0 || years <= 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("cagr: start and years must be positive"), nil)
		}
		return common.MakeUDFSuccessResult(math.Pow(end/start, 1/years)-1, nil)
	})
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return fmt.Errorf("bad arguments")
}

// RegisterFutureValue registers future_value, the value of a principal after
// periods of compounding at a rate per period.
func RegisterFutureValue() gojq.CompilerOption {
	return gojq.WithFunction("future_value", 3, 3, func(v any, args []any) any {
		principal, err1 := numArg(args, 0, "future_value")
		rate, err2 := numArg(args, 1, "future_value")
		periods, err3 := numArg(args, 2, "future_value")
		if err1 != nil || err2 != nil || err3 != nil {
			return common.MakeUDFErrorResult(firstErr(err1, err2, err3), nil)
		}
		return common.MakeUDFSuccessResult(principal*math.Pow(1+rate, periods), nil)
	})
}

// RegisterPresentValue registers present_value, the principal that would grow
// to a target after periods at a rate per period.
func RegisterPresentValue() gojq.CompilerOption {
	return gojq.WithFunction("present_value", 3, 3, func(v any, args []any) any {
		target, err1 := numArg(args, 0, "present_value")
		rate, err2 := numArg(args, 1, "present_value")
		periods, err3 := numArg(args, 2, "present_value")
		if err1 != nil || err2 != nil || err3 != nil {
			return common.MakeUDFErrorResult(firstErr(err1, err2, err3), nil)
		}
		return common.MakeUDFSuccessResult(target/math.Pow(1+rate, periods), nil)
	})
}

// RegisterMonthlyPayment registers monthly_payment, the fixed monthly payment
// that amortizes a loan of principal over months at an annual rate.
func RegisterMonthlyPayment() gojq.CompilerOption {
	return gojq.WithFunction("monthly_payment", 3, 3, func(v any, args []any) any {
		principal, err1 := numArg(args, 0, "monthly_payment")
		annualRate, err2 := numArg(args, 1, "monthly_payment")
		months, err3 := numArg(args, 2, "monthly_payment")
		if err1 != nil || err2 != nil || err3 != nil {
			return common.MakeUDFErrorResult(firstErr(err1, err2, err3), nil)
		}
		if months <= 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("monthly_payment: months must be positive"), nil)
		}
		r := annualRate / 12
		if r == 0 {
			return common.MakeUDFSuccessResult(principal/months, nil)
		}
		return common.MakeUDFSuccessResult(principal*r*math.Pow(1+r, months)/(math.Pow(1+r, months)-1), nil)
	})
}

// RegisterCompoundInterest registers compound_interest, the interest earned on
// a principal over periods at a rate per period (future value minus principal).
func RegisterCompoundInterest() gojq.CompilerOption {
	return gojq.WithFunction("compound_interest", 3, 3, func(v any, args []any) any {
		principal, err1 := numArg(args, 0, "compound_interest")
		rate, err2 := numArg(args, 1, "compound_interest")
		periods, err3 := numArg(args, 2, "compound_interest")
		if err1 != nil || err2 != nil || err3 != nil {
			return common.MakeUDFErrorResult(firstErr(err1, err2, err3), nil)
		}
		return common.MakeUDFSuccessResult(principal*(math.Pow(1+rate, periods)-1), nil)
	})
}

// RegisterSimpleInterest registers simple_interest, interest computed on the
// principal only: principal * rate * years.
func RegisterSimpleInterest() gojq.CompilerOption {
	return gojq.WithFunction("simple_interest", 3, 3, func(v any, args []any) any {
		principal, err1 := numArg(args, 0, "simple_interest")
		rate, err2 := numArg(args, 1, "simple_interest")
		years, err3 := numArg(args, 2, "simple_interest")
		if err1 != nil || err2 != nil || err3 != nil {
			return common.MakeUDFErrorResult(firstErr(err1, err2, err3), nil)
		}
		return common.MakeUDFSuccessResult(principal*rate*years, nil)
	})
}

// RegisterRuleOf72 registers rule_of_72, the years for a value to double at an
// annual rate, by the rule-of-72 approximation.
func RegisterRuleOf72() gojq.CompilerOption {
	return gojq.WithFunction("rule_of_72", 1, 1, func(v any, args []any) any {
		rate, err := numArg(args, 0, "rule_of_72")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		if rate <= 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("rule_of_72: rate must be positive"), nil)
		}
		return common.MakeUDFSuccessResult(72/(rate*100), nil)
	})
}

// RegisterAnnualYield registers annual_yield, the effective annual rate when a
// nominal annual rate is compounded compoundingPerYear times a year.
func RegisterAnnualYield() gojq.CompilerOption {
	return gojq.WithFunction("annual_yield", 2, 2, func(v any, args []any) any {
		nominal, err1 := numArg(args, 0, "annual_yield")
		perYear, err2 := numArg(args, 1, "annual_yield")
		if err1 != nil || err2 != nil {
			return common.MakeUDFErrorResult(firstErr(err1, err2), nil)
		}
		if perYear <= 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("annual_yield: compounding periods must be positive"), nil)
		}
		return common.MakeUDFSuccessResult(math.Pow(1+nominal/perYear, perYear)-1, nil)
	})
}
