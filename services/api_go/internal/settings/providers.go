package settings

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"unicode"
)

const (
	ProviderSiliconFlow = "siliconflow"
	ProviderAliyun      = "aliyun"
	ProviderDeepSeek    = "deepseek"
	AliyunKey           = "aliyun_api_key"
	SiliconFlowKey      = "siliconflow_api_key"
	DeepSeekKey         = "deepseek_api_key"
	ASRModel            = "asr_model"
	DefaultASRModel     = "FunAudioLLM/SenseVoiceSmall"
	providerPolicy      = "asr_provider_policy"
	disabledValue       = "\x00"
)

var ErrInvalidProviders = errors.New("invalid ASR provider settings")

var providerDefinitions = [...]struct {
	id, key, model string
}{
	{ProviderSiliconFlow, SiliconFlowKey, DefaultASRModel},
	{ProviderAliyun, AliyunKey, "paraformer-v2"},
	{ProviderDeepSeek, DeepSeekKey, "DeepSeek ASR"},
}

type ProviderStatus struct {
	ID         string `json:"id"`
	Enabled    bool   `json:"enabled"`
	Configured bool   `json:"configured"`
	Available  bool   `json:"available"`
	Model      string `json:"model"`
}

type ProviderConfig struct {
	ProviderStatus
	APIKey string `json:"-"`
}

type ProviderStatusList struct {
	Items                    []ProviderStatus `json:"items"`
	DefaultProvider          string           `json:"defaultProvider"`
	EffectiveDefaultProvider string           `json:"effectiveDefaultProvider"`
	AllowUserSelection       bool             `json:"allowUserSelection"`
	AliyunConfigured         bool             `json:"aliyunConfigured"`
	AliyunAvailable          bool             `json:"aliyunAvailable"`
	SiliconFlowConfigured    bool             `json:"siliconFlowConfigured"`
	ASRModel                 string           `json:"asrModel"`
}

type ProviderOption struct {
	ID string `json:"id"`
}

type ProviderSelection struct {
	DefaultProvider    string           `json:"defaultProvider"`
	AllowUserSelection bool             `json:"allowUserSelection"`
	Providers          []ProviderOption `json:"providers"`
}

type ProviderSettings struct {
	Status        ProviderStatusList
	configs       map[string]ProviderConfig
	policy        providerPolicyValue
	policyManaged bool
}

type providerPolicyValue struct {
	DefaultProvider    string   `json:"defaultProvider"`
	AllowUserSelection bool     `json:"allowUserSelection"`
	EnabledProviders   []string `json:"enabledProviders"`
}

type ProviderUpdate struct {
	AliyunAPIKey       *string   `json:"aliyunApiKey"`
	SiliconFlowAPIKey  *string   `json:"siliconFlowApiKey"`
	DeepSeekAPIKey     *string   `json:"deepseekApiKey"`
	ASRModel           *string   `json:"asrModel"`
	DefaultProvider    *string   `json:"defaultProvider"`
	AllowUserSelection *bool     `json:"allowUserSelection"`
	EnabledProviders   *[]string `json:"enabledProviders"`
}

func ValidProvider(id string) bool {
	for _, definition := range providerDefinitions {
		if definition.id == id {
			return true
		}
	}
	return false
}

func (p ProviderSettings) Config(id string) ProviderConfig {
	return p.configs[id]
}

func (p ProviderSettings) Selection() ProviderSelection {
	options := make([]ProviderOption, 0, len(p.Status.Items))
	for _, item := range p.Status.Items {
		if item.Enabled && item.Available {
			options = append(options, ProviderOption{ID: item.ID})
		}
	}
	return ProviderSelection{
		DefaultProvider: p.Status.EffectiveDefaultProvider, AllowUserSelection: p.Status.AllowUserSelection, Providers: options,
	}
}

func (p ProviderSettings) Allows(requested string) bool {
	if requested == "" {
		// Older API clients could queue a default transcription before a key was provisioned.
		return p.Status.EffectiveDefaultProvider != "" || !p.policyManaged
	}
	item := p.Config(requested)
	return p.Status.AllowUserSelection && item.Enabled && item.Available
}

func (s *Service) Providers(ctx context.Context, aliyunAvailable bool) (ProviderSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.readProviders(ctx, s.db, aliyunAvailable)
}

type providerReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (s *Service) readProviders(ctx context.Context, reader providerReader, aliyunAvailable bool) (ProviderSettings, error) {
	result := ProviderSettings{policy: providerPolicyValue{
		AllowUserSelection: true, EnabledProviders: []string{ProviderSiliconFlow, ProviderAliyun, ProviderDeepSeek},
	}}
	values := map[string]string{
		AliyunKey: s.fallback[AliyunKey], SiliconFlowKey: s.fallback[SiliconFlowKey],
		DeepSeekKey: s.fallback[DeepSeekKey], ASRModel: s.fallback[ASRModel],
	}
	rows, err := reader.QueryContext(ctx, `SELECT name, encrypted_value FROM runtime_settings WHERE name IN (?, ?, ?, ?, ?)`,
		AliyunKey, SiliconFlowKey, DeepSeekKey, ASRModel, providerPolicy)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var encrypted []byte
		if err := rows.Scan(&name, &encrypted); err != nil {
			return result, err
		}
		value, err := s.decrypt(name, encrypted)
		if err != nil {
			return result, err
		}
		if name == providerPolicy {
			if err := json.Unmarshal([]byte(value), &result.policy); err != nil {
				return result, err
			}
			result.policyManaged = true
		} else if value == disabledValue {
			if name != ASRModel {
				values[name] = ""
			}
		} else {
			values[name] = value
		}
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	result.Status = ProviderStatusList{
		Items: make([]ProviderStatus, 0, len(providerDefinitions)), DefaultProvider: result.policy.DefaultProvider,
		AllowUserSelection: result.policy.AllowUserSelection, AliyunAvailable: aliyunAvailable,
		AliyunConfigured: values[AliyunKey] != "", SiliconFlowConfigured: values[SiliconFlowKey] != "",
		ASRModel: values[ASRModel],
	}
	if result.Status.ASRModel == "" {
		result.Status.ASRModel = DefaultASRModel
	}
	result.configs = make(map[string]ProviderConfig, len(providerDefinitions))
	for _, definition := range providerDefinitions {
		key := values[definition.key]
		item := ProviderStatus{
			ID: definition.id, Model: definition.model,
			Enabled: slices.Contains(result.policy.EnabledProviders, definition.id), Configured: key != "",
			Available: key != "" && (definition.id != ProviderAliyun || aliyunAvailable),
		}
		if definition.id == ProviderSiliconFlow {
			item.Model = result.Status.ASRModel
		}
		result.Status.Items = append(result.Status.Items, item)
		result.configs[item.ID] = ProviderConfig{ProviderStatus: item, APIKey: key}
		if item.Enabled && item.Available && result.Status.EffectiveDefaultProvider == "" &&
			(result.policy.DefaultProvider == "" || result.policy.DefaultProvider == item.ID) {
			result.Status.EffectiveDefaultProvider = item.ID
		}
	}
	return result, nil
}

func (update ProviderUpdate) policyChanged() bool {
	return update.DefaultProvider != nil || update.EnabledProviders != nil || update.AllowUserSelection != nil
}

func (s *Service) UpdateProviders(ctx context.Context, update ProviderUpdate, actorID string, aliyunAvailable bool) (ProviderStatusList, error) {
	values := make(map[string]string, 4)
	for name, input := range map[string]*string{
		AliyunKey: update.AliyunAPIKey, SiliconFlowKey: update.SiliconFlowAPIKey, DeepSeekKey: update.DeepSeekAPIKey, ASRModel: update.ASRModel,
	} {
		if input == nil {
			continue
		}
		value := strings.TrimSpace(*input)
		maxLength := 4096
		if name == ASRModel {
			maxLength = 256
		}
		if len(value) > maxLength || strings.ContainsFunc(value, func(r rune) bool {
			return unicode.IsSpace(r) || unicode.IsControl(r) || (name != ASRModel && r > unicode.MaxASCII)
		}) {
			return ProviderStatusList{}, ErrInvalidProviders
		}
		if value == "" {
			value = disabledValue
		}
		values[name] = value
	}
	if len(values) == 0 && !update.policyChanged() {
		return ProviderStatusList{}, ErrInvalidProviders
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ProviderStatusList{}, err
	}
	defer tx.Rollback()
	if update.policyChanged() {
		current, err := s.readProviders(ctx, tx, aliyunAvailable)
		if err != nil {
			return ProviderStatusList{}, err
		}
		policy := current.policy
		if update.DefaultProvider != nil {
			policy.DefaultProvider = *update.DefaultProvider
		}
		if update.AllowUserSelection != nil {
			policy.AllowUserSelection = *update.AllowUserSelection
		}
		if update.EnabledProviders != nil {
			if len(*update.EnabledProviders) > len(providerDefinitions) {
				return ProviderStatusList{}, ErrInvalidProviders
			}
			policy.EnabledProviders = []string{}
			for _, id := range *update.EnabledProviders {
				if !ValidProvider(id) || slices.Contains(policy.EnabledProviders, id) {
					return ProviderStatusList{}, ErrInvalidProviders
				}
				policy.EnabledProviders = append(policy.EnabledProviders, id)
			}
		}
		if policy.DefaultProvider != "" && !slices.Contains(policy.EnabledProviders, policy.DefaultProvider) {
			return ProviderStatusList{}, ErrInvalidProviders
		}
		encoded, err := json.Marshal(policy)
		if err != nil {
			return ProviderStatusList{}, err
		}
		values[providerPolicy] = string(encoded)
	}
	for name, value := range values {
		if err := s.put(ctx, tx, name, value, actorID); err != nil {
			return ProviderStatusList{}, err
		}
	}
	current, err := s.readProviders(ctx, tx, aliyunAvailable)
	if err != nil {
		return ProviderStatusList{}, err
	}
	if err := tx.Commit(); err != nil {
		return ProviderStatusList{}, err
	}
	return current.Status, nil
}
