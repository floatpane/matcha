package passwordstrength

import passwordvalidator "github.com/wagslane/go-password-validator"

type LibMeter struct{}

func NewLibMeter() LibMeter {
	return LibMeter{}
}

func (m LibMeter) Strength(password string) Strength {
	entropy := passwordvalidator.GetEntropy(password)
	switch {
	case entropy >= strongEntropyBits:
		return Strong
	case entropy >= mediumEntropyBits:
		return Medium
	default:
		return Weak
	}
}
