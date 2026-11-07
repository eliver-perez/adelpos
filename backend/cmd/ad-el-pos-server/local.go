package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
	"local/ad-el-pos/server/internal/administration"
)

func prepareSetup(filename string) error {
	destination := filepath.Join(filepath.Dir(filename), "provisioning-token")
	if existing, err := os.ReadFile(destination); err == nil {
		if len(strings.TrimSpace(string(existing))) != 43 {
			return errors.New("el archivo de aprovisionamiento existente no es válido; requiere revisión local")
		}
		fmt.Println("El aprovisionamiento local ya está preparado; no se modificó el secreto.")
		return nil
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("no se pudo crear el archivo privado de aprovisionamiento")
	}
	defer file.Close()
	if _, err := file.WriteString(administration.NewToken() + "\n"); err != nil {
		return err
	}
	fmt.Println("Aprovisionamiento preparado. El secreto permanece en el archivo privado local.")
	return nil
}

func provisionSupport(ctx context.Context, service *administration.Service, reset bool) error {
	terminal, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil || !term.IsTerminal(int(terminal.Fd())) {
		return errors.New("el aprovisionamiento de sa requiere una terminal local interactiva")
	}
	defer terminal.Close()
	if reset {
		fmt.Fprint(terminal, "Para autorizar el restablecimiento local escribe RESET sa: ")
		confirmation, err := bufio.NewReader(terminal).ReadString('\n')
		if err != nil || strings.TrimSpace(confirmation) != "RESET sa" {
			return errors.New("restablecimiento cancelado")
		}
	}
	fmt.Fprint(terminal, "Contraseña privada de sa (mínimo 12 caracteres): ")
	password, err := term.ReadPassword(int(terminal.Fd()))
	fmt.Fprintln(terminal)
	if err != nil {
		return errors.New("no se pudo leer la contraseña local")
	}
	defer clear(password)
	fmt.Fprint(terminal, "Repite la contraseña: ")
	confirmation, err := term.ReadPassword(int(terminal.Fd()))
	fmt.Fprintln(terminal)
	defer clear(confirmation)
	if err != nil || string(password) != string(confirmation) {
		return errors.New("las contraseñas no coinciden")
	}
	if err := service.ProvisionSupport(ctx, string(password), reset); err != nil {
		return err
	}
	fmt.Fprintln(terminal, "Credencial reservada guardada y operación auditada. No se mostró ni registró el secreto.")
	return nil
}

func createLocalEnrollment(ctx context.Context, service *administration.Service, filename string) error {
	code, err := service.CreateLocalEnrollment(ctx)
	if err != nil {
		return errors.New("no se pudo emitir la autorización local; comprueba migraciones y PostgreSQL")
	}
	destination := filepath.Join(filepath.Dir(filename), "enrollment-code")
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return errors.New("no se pudo guardar la autorización local")
	}
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return err
	}
	if _, err := file.WriteString(code + "\n"); err != nil {
		return err
	}
	fmt.Println("Autorización local creada en un archivo privado; expira en 10 minutos. Abre el escritorio con dev.sh.")
	return nil
}
