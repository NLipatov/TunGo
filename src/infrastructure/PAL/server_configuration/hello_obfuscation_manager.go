package server_configuration

type HelloObfuscationManager interface {
	PrepareHelloObfuscationKeys() error
}

type DefaultHelloObfuscationManager struct {
	config  *Configuration
	manager ServerConfigurationManager
}

func NewDefaultHelloObfuscationManager(
	config *Configuration,
	manager ServerConfigurationManager,
) HelloObfuscationManager {
	return &DefaultHelloObfuscationManager{
		config:  config,
		manager: manager,
	}
}

func (d *DefaultHelloObfuscationManager) PrepareHelloObfuscationKeys() error {
	if !d.hasValidObfuscationKeys() {
		err := d.manager.InjectHelloObfuscationKeys()
		if err != nil {
			return err
		}
	}

	return nil
}

func (d *DefaultHelloObfuscationManager) hasValidObfuscationKeys() bool {
	if len(d.config.UDPSettings.HelloMasking.AEAD) > 0 && len(d.config.UDPSettings.HelloMasking.HMAC) > 0 &&
		len(d.config.TCPSettings.HelloMasking.AEAD) > 0 && len(d.config.TCPSettings.HelloMasking.HMAC) > 0 {
		return true
	}

	return false
}
