package cursorproto

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// ModelDetail is the subset of agent.v1.ModelDetails that the plugin publishes.
type ModelDetail struct {
	ID          string
	DisplayName string
	Aliases     []string
	MaxMode     bool
}

func DecodeModels(raw []byte) ([]string, error) {
	details, err := DecodeModelDetails(raw)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(details))
	for _, detail := range details {
		ids = append(ids, detail.ID)
	}
	return ids, nil
}

// DecodeModelDetails decodes GetUsableModelsResponse without dropping display metadata.
func DecodeModelDetails(raw []byte) ([]ModelDetail, error) {
	response, err := newMessage("GetUsableModelsResponse")
	if err != nil {
		return nil, err
	}
	if err := proto.Unmarshal(raw, response); err != nil {
		return nil, fmt.Errorf("decode Cursor models response: %w", err)
	}
	modelsField, err := requireField(response, "models")
	if err != nil {
		return nil, err
	}
	models := response.Get(modelsField).List()
	details := make([]ModelDetail, 0, models.Len())
	for index := range models.Len() {
		model := models.Get(index).Message()
		idField, err := requireField(model, "model_id")
		if err != nil {
			return nil, err
		}
		id := strings.TrimSpace(model.Get(idField).String())
		if id == "" {
			continue
		}
		detail := ModelDetail{ID: id}
		detail.DisplayName = optionalString(model, "display_name")
		if aliases := field(model, "aliases"); aliases != nil && aliases.IsList() {
			list := model.Get(aliases).List()
			for aliasIndex := range list.Len() {
				if alias := strings.TrimSpace(list.Get(aliasIndex).String()); alias != "" {
					detail.Aliases = append(detail.Aliases, alias)
				}
			}
		}
		if maxMode := field(model, "max_mode"); maxMode != nil && maxMode.Kind() == protoreflect.BoolKind {
			detail.MaxMode = model.Get(maxMode).Bool()
		}
		details = append(details, detail)
	}
	return details, nil
}

func optionalString(message protoreflect.Message, name protoreflect.Name) string {
	descriptor := field(message, name)
	if descriptor == nil || descriptor.Kind() != protoreflect.StringKind || descriptor.IsList() {
		return ""
	}
	return strings.TrimSpace(message.Get(descriptor).String())
}
