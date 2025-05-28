package core

import (
	"fmt"
	"log/slog"
	"os"
	"path"
	"text/template"

	"machinecfg/internal/core/domain"
	"machinecfg/internal/core/domain/ports"
)

type Engine struct {
	args         *domain.ConfigurationArgs
	output       ports.Output
	template     *template.Template
	templateFile string
}

func NewEngine(args *domain.ConfigurationArgs, templateFile string, output ports.Output) *Engine {
	var tmpl = template.Must(template.New(templateFile).ParseFiles(templateFile))

	slog.Debug("NewEngine", "templates", tmpl.DefinedTemplates())

	return &Engine{
		args:         args,
		template:     tmpl,
		templateFile: path.Base(templateFile),
		output:       output,
	}
}

func (e *Engine) PrintYAMLTemplates(machines []domain.MachineInfo) {
	for _, machine := range machines {
		err := e.printYAMLTemplate(&machine)
		if err != nil {
			slog.Error("PrintYAMLTemplates", "message", err.Error())
		}
	}
}

func (e *Engine) printYAMLTemplate(machine *domain.MachineInfo) (err error) {
	var fd *os.File
	fileName := fmt.Sprintf("%s.yaml", machine.Hostname)

	if ValidateMachineInfo(machine) {
		slog.Debug("printYAMLTemplates", "machine", machine)
		fd, err = e.output.GetFileDescriptor(&fileName)
		if err == nil {
			defer e.output.Close(fd)
			err = e.template.ExecuteTemplate(fd, e.templateFile, machine)
		}
	}

	return err
}
