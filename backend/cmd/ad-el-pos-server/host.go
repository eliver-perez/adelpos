package main

// hostOptions son los indicadores de los comandos de la instalación de Windows (etapa 9E): host, backup,
// restore-check y restore. Se declaran en todas las plataformas para que el análisis de argumentos sea el
// mismo; fuera de Windows esos comandos no existen (host_other.go).
type hostOptions struct {
	dataDir    string
	pgBin      string
	file       string
	watchStdin bool
	pause      bool
}
