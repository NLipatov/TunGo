package bubble_tea

import "tungo/internal/product"

func productLabel() string {
	return product.Name + " [" + product.Version + "]"
}
