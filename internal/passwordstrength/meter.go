package passwordstrength

type Strength string

const (
	Weak   Strength = "weak"
	Medium Strength = "medium"
	Strong Strength = "strong"
)

const (
	weakEntropyBits   = 30
	mediumEntropyBits = 50
	strongEntropyBits = 70
)

type Meter interface {
	Strength(password string) Strength
}
