package internal

import (
	"fmt"
	"github.com/tomwright/grpc-simple-js/internal/mapping"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
	"log"
	"path/filepath"
	"strings"
	"text/template"
)

type messageTemplateData struct {
	Message *protogen.Message
	Package string
	Prefix  string
}

// todo : remove comma from last field in exported type
var messageTemplate = template.Must(template.New("message").
	Funcs(defaultFuncMap.toMap()).
	Parse(`
{{- $package := .Package -}}
export type {{ .Prefix }}{{ messageName .Message }} = {
{{- range .Message.Fields }}
    {{ fieldName . }}?: {{ fieldType . $package }},
{{- end }}
}

`))

type enumTemplateData struct {
	Enum    *protogen.Enum
	Package string
	Prefix  string
}

// todo : remove comma from last field in exported enum
var enumTemplate = template.Must(template.New("enum").
	Funcs(defaultFuncMap.toMap()).
	Parse(`
export enum {{.Prefix}}{{ enumName .Enum }} {
{{- range .Enum.Values }}
    {{ enumValueName . }} = {{ enumValue . }},
{{- end }}
}

`))

func (p *Runner) writeMessages(messages []*protogen.Message, out *protogen.GeneratedFile, currentPkg string) {
	for _, m := range messages {
		data := messageTemplateData{
			Message: m,
			Package: currentPkg,
			Prefix:  mapping.DescriptorPrefix(m.Desc),
		}
		if err := messageTemplate.Execute(out, data); err != nil {
			log.Fatalf("messageTemplate.Execute failed: %s", err)
		}

		p.writeMessages(m.Messages, out, currentPkg)
		p.writeEnums(m.Enums, out, currentPkg)
	}
}

func (p *Runner) writeEnums(enums []*protogen.Enum, out *protogen.GeneratedFile, currentPkg string) {
	for _, m := range enums {
		data := enumTemplateData{
			Enum:    m,
			Package: currentPkg,
			Prefix:  mapping.DescriptorPrefix(m.Desc),
		}
		if err := enumTemplate.Execute(out, data); err != nil {
			log.Fatalf("enumTemplate.Execute failed: %s", err)
		}
	}
}

func (p *Runner) generateTypes(plugin *protogen.Plugin) error {
	for _, f := range plugin.Files {
		// Create the output file
		outputDir := filepath.Dir(f.Desc.Path())
		outputFile := strings.TrimSuffix(filepath.Base(f.Desc.Path()), filepath.Ext(f.Desc.Path())) + "_types_sjs.ts"
		outputPath := fmt.Sprintf("%s/%s", outputDir, outputFile)
		out := plugin.NewGeneratedFile(outputPath, "")

		_, _ = out.Write([]byte(`// File auto-generated by protoc-gen-simple-js
`))

		if err := p.generateTypesImports(f, out); err != nil {
			log.Fatalf("could not add required imports: %s", err)
		}

		currentPackage := mapping.DescriptorPackage(f.Desc, "types")

		p.writeMessages(f.Messages, out, currentPackage)
		p.writeEnums(f.Enums, out, currentPackage)

	}

	return nil
}

func (p *Runner) generateTypesImports(f *protogen.File, out *protogen.GeneratedFile) error {
	currentPath := f.Desc.Path()

	// required is a list of required paths to import
	required := make([]*requiredImport, 0)
	addRequired := func(descriptor protoreflect.Descriptor) {
		var requiredFile protoreflect.FileDescriptor

		switch d := descriptor.(type) {
		case protoreflect.MessageDescriptor:
			requiredFile = p.messageFiles[d.FullName()]
		case protoreflect.EnumDescriptor:
			requiredFile = p.enumFiles[d.FullName()]
		default:
			log.Println("addRequiredImports: skipping unhandled descriptor type")
			return
		}

		for _, r := range required {
			if r.FileDesc == requiredFile {
				return
			}
		}

		r := &requiredImport{
			FileDesc:     requiredFile,
			ImportName:   mapping.PkgToImportPkg(mapping.DescriptorPackage(requiredFile, "types")),
			RelativePath: mapping.ProtoToSimpleJS(mapping.RelativePathBetweenPaths(currentPath, requiredFile.Path()), false, "_types"),
		}

		if r.RelativePath == "" {
			return
		}

		required = append(required, r)
	}

	for _, m := range f.Messages {
		for _, f := range m.Fields {
			switch f.Desc.Kind() {
			case protoreflect.MessageKind:
				addRequired(f.Desc.Message())
			case protoreflect.EnumKind:
				addRequired(f.Desc.Enum())
			default:
				continue
			}
		}
	}

	if err := importsTemplate.Execute(out, required); err != nil {
		return fmt.Errorf("could not execute imports template: %w", err)
	}

	return nil
}
