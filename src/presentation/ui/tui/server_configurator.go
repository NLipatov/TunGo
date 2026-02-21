package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"tungo/application/confgen"
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/presentation/ui/tui/internal/ui/contracts/selector"
	"tungo/presentation/ui/tui/internal/ui/value_objects"
)

const (
	startServerOption string = labelStartServer
	addClientOption   string = labelAddServerPeer
	manageClients     string = labelManageClients
)

type serverConfigurator struct {
	manager         server.ConfigurationManager
	optionsSet      [3]string
	selectorFactory selector.Factory
	notice          string
}

type clientConfigGenerator interface {
	Generate() (*clientConfiguration.Configuration, error)
}

var (
	newServerClientConfigGenerator = func(manager server.ConfigurationManager) clientConfigGenerator {
		return confgen.NewGenerator(manager, &primitives.DefaultKeyDeriver{})
	}
	marshalServerClientConfiguration = func(v any) ([]byte, error) {
		return json.MarshalIndent(v, "", "  ")
	}
	resolveServerConfigDir = func() (string, error) {
		configPath, err := server.NewServerResolver().Resolve()
		if err != nil {
			return "", err
		}
		return filepath.Dir(configPath), nil
	}
	writeServerClientConfigurationFile = func(clientID int, data []byte) (string, error) {
		dir, err := resolveServerConfigDir()
		if err != nil {
			return "", fmt.Errorf("failed to resolve server config directory: %w", err)
		}
		path := filepath.Join(dir, fmt.Sprintf("client_configuration.json.%d", clientID))
		return path, os.WriteFile(path, data, 0600)
	}
)

type serverFlowState int

const (
	serverStateSelectOption serverFlowState = iota
	serverStateGenerateClient
	serverStateManageClients
)

var errNoAllowedPeers = errors.New("no allowed peers configured")

func newServerConfigurator(manager server.ConfigurationManager, selectorFactory selector.Factory) *serverConfigurator {
	return &serverConfigurator{
		manager:         manager,
		optionsSet:      [3]string{startServerOption, addClientOption, manageClients},
		selectorFactory: selectorFactory,
	}
}

func (s *serverConfigurator) Configure() error {
	return s.configureFromState(serverStateSelectOption)
}

func (s *serverConfigurator) configureFromState(state serverFlowState) error {
	for {
		switch state {
		case serverStateSelectOption:
			option, optionErr := s.selectOption()
			if optionErr != nil {
				if errors.Is(optionErr, selector.ErrNavigateBack) {
					return ErrBackToModeSelection
				}
				if errors.Is(optionErr, selector.ErrUserExit) {
					return ErrUserExit
				}
				return optionErr
			}
			s.notice = ""

			switch option {
			case startServerOption:
				return nil
			case addClientOption:
				state = serverStateGenerateClient
			case manageClients:
				state = serverStateManageClients
			default:
				return fmt.Errorf("invalid option: %s", option)
			}

		case serverStateGenerateClient:
			gen := newServerClientConfigGenerator(s.manager)
			conf, err := gen.Generate()
			if err != nil {
				return err
			}
			data, err := marshalServerClientConfiguration(conf)
			if err != nil {
				return fmt.Errorf("failed to marshal client configuration: %w", err)
			}
			path, fileErr := writeServerClientConfigurationFile(conf.ClientID, data)
			if fileErr != nil {
				return fmt.Errorf("failed to save client configuration: %w", fileErr)
			}
			s.notice = fmt.Sprintf("Client configuration saved to %s", path)
			state = serverStateSelectOption
		case serverStateManageClients:
			selectedPeer, selectErr := s.selectManagedPeer()
			if selectErr != nil {
				switch {
				case errors.Is(selectErr, selector.ErrNavigateBack):
					state = serverStateSelectOption
					continue
				case errors.Is(selectErr, selector.ErrUserExit):
					return ErrUserExit
				case errors.Is(selectErr, errNoAllowedPeers):
					s.notice = "No clients configured yet."
					state = serverStateSelectOption
					continue
				default:
					return selectErr
				}
			}

			nextEnabled := !selectedPeer.Enabled
			if err := s.manager.SetAllowedPeerEnabled(selectedPeer.ClientID, nextEnabled); err != nil {
				return fmt.Errorf(
					"failed to update client #%d status: %w",
					selectedPeer.ClientID,
					err,
				)
			}
			status := "disabled"
			if nextEnabled {
				status = "enabled"
			}
			s.notice = fmt.Sprintf(
				"Client #%d %s is now %s.",
				selectedPeer.ClientID,
				serverPeerDisplayName(selectedPeer),
				status,
			)
			state = serverStateManageClients

		default:
			return fmt.Errorf("unknown server flow state: %d", state)
		}
	}
}

func (s *serverConfigurator) selectOption() (string, error) {
	label := "Choose an option"
	if strings.TrimSpace(s.notice) != "" {
		label = label + "\n\n" + s.notice
	}
	tuiSelector, selectorErr := s.selectorFactory.NewTuiSelector(
		label,
		s.optionsSet[:],
		value_objects.NewDefaultColor(),
		value_objects.NewTransparentColor(),
	)
	if selectorErr != nil {
		return "", selectorErr
	}

	selectedOption, selectedOptionErr := tuiSelector.SelectOne()
	if selectedOptionErr != nil {
		return "", selectedOptionErr
	}
	return selectedOption, nil
}

func (s *serverConfigurator) selectManagedPeer() (server.AllowedPeer, error) {
	peers, err := s.manager.ListAllowedPeers()
	if err != nil {
		return server.AllowedPeer{}, err
	}
	if len(peers) == 0 {
		return server.AllowedPeer{}, errNoAllowedPeers
	}

	options := make([]string, 0, len(peers))
	labelToPeer := make(map[string]server.AllowedPeer, len(peers))
	for _, peer := range peers {
		label := serverPeerOptionLabel(peer)
		options = append(options, label)
		labelToPeer[label] = peer
	}

	tuiSelector, selectorErr := s.selectorFactory.NewTuiSelector(
		"Select client to enable/disable",
		options,
		value_objects.NewDefaultColor(),
		value_objects.NewTransparentColor(),
	)
	if selectorErr != nil {
		return server.AllowedPeer{}, selectorErr
	}

	selectedOption, selectedOptionErr := tuiSelector.SelectOne()
	if selectedOptionErr != nil {
		return server.AllowedPeer{}, selectedOptionErr
	}

	peer, ok := labelToPeer[selectedOption]
	if !ok {
		return server.AllowedPeer{}, fmt.Errorf("unknown managed client option: %s", selectedOption)
	}
	return peer, nil
}

func serverPeerDisplayName(peer server.AllowedPeer) string {
	name := strings.TrimSpace(peer.Name)
	if name == "" {
		return fmt.Sprintf("client-%d", peer.ClientID)
	}
	return name
}

func serverPeerOptionLabel(peer server.AllowedPeer) string {
	status := "disabled"
	if peer.Enabled {
		status = "enabled"
	}
	return fmt.Sprintf(
		"#%d %s [%s]",
		peer.ClientID,
		serverPeerDisplayName(peer),
		status,
	)
}
