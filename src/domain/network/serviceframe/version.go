package serviceframe

type Version uint8

const (
	V1 Version = 1
)

func (v Version) IsValid() bool {
	if v == V1 {
		return true
	}
	return false
}
