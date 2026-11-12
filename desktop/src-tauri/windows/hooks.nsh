; AD-EL POS (etapa 9E): extensiones del instalador NSIS de Windows (bundle.windows.nsis.installerHooks).
; Se guarda en UTF-8 con BOM: makensis solo así lee bien los textos con acentos de los accesos directos.
;
; Reglas de la especificación (0-B y §4): la desinstalación quita la aplicación y deja los datos; reinstalar o
; actualizar los conserva. Ningún paso de este archivo borra, mueve ni modifica la carpeta de datos del
; restaurante (%LOCALAPPDATA%\AD-EL POS): solo detiene PostgreSQL en orden si quedó activo.

!define ADEL_DATA_DIR "$LOCALAPPDATA\${PRODUCTNAME}"
!define ADEL_SERVER_EXE "ad-el-pos-server.exe"
!define ADEL_BACKUP_SHORTCUT "Crear respaldo de ${PRODUCTNAME}.lnk"
!define ADEL_BACKUPS_SHORTCUT "Respaldos de ${PRODUCTNAME}.lnk"

; Espera a que terminen el servidor local y PostgreSQL de esta instalación. La plantilla la llama después de
; cerrar la aplicación abierta: al cerrarse, la aplicación pide a su servidor que se detenga y este detiene
; PostgreSQL. Un ejecutable en uso no se puede abrir para escritura, así que se reintenta cada medio segundo
; hasta 40 s. Si después PostgreSQL sigue activo (por ejemplo, tras un cierre anómalo) se detiene con
; pg_ctl en modo fast, que conserva los datos.
!macro AdelWaitLocalServer
  !define AdelWaitID ${__LINE__}
  Push $0
  Push $1
  StrCpy $1 0
  ${If} ${FileExists} "$INSTDIR\${ADEL_SERVER_EXE}"
    adel_wait_server_${AdelWaitID}:
      ClearErrors
      FileOpen $0 "$INSTDIR\${ADEL_SERVER_EXE}" a
      ${IfNot} ${Errors}
        FileClose $0
        Goto adel_server_done_${AdelWaitID}
      ${EndIf}
      IntOp $1 $1 + 1
      ${If} $1 < 80
        Sleep 500
        Goto adel_wait_server_${AdelWaitID}
      ${EndIf}
    adel_server_done_${AdelWaitID}:
  ${EndIf}
  ${If} ${FileExists} "${ADEL_DATA_DIR}\postgres\postmaster.pid"
  ${AndIf} ${FileExists} "$INSTDIR\postgresql\bin\pg_ctl.exe"
    nsExec::Exec '"$INSTDIR\postgresql\bin\pg_ctl.exe" stop -D "${ADEL_DATA_DIR}\postgres" -m fast -w -t 60'
    Pop $0
  ${EndIf}
  Pop $1
  Pop $0
  !undef AdelWaitID
!macroend

; Accesos directos de mantenimiento junto al de la aplicación: crear un respaldo (abre una ventana con el
; resultado y espera Entrar) y abrir la carpeta de respaldos para copiarlos a otra unidad.
!macro NSIS_HOOK_POSTINSTALL
  CreateDirectory "${ADEL_DATA_DIR}\backups"
  CreateDirectory "$SMPROGRAMS\$AppStartMenuFolder"
  CreateShortcut "$SMPROGRAMS\$AppStartMenuFolder\${ADEL_BACKUP_SHORTCUT}" "$INSTDIR\${ADEL_SERVER_EXE}" "backup --pause" "$INSTDIR\${MAINBINARYNAME}.exe" 0
  CreateShortcut "$SMPROGRAMS\$AppStartMenuFolder\${ADEL_BACKUPS_SHORTCUT}" "${ADEL_DATA_DIR}\backups"
!macroend

; Antes de desinstalar se quitan esos accesos directos (la plantilla borra la carpeta del menú Inicio solo si
; queda vacía). La carpeta de datos y sus respaldos se conservan.
!macro NSIS_HOOK_PREUNINSTALL
  !insertmacro MUI_STARTMENU_GETFOLDER Application $AppStartMenuFolder
  Delete "$SMPROGRAMS\$AppStartMenuFolder\${ADEL_BACKUP_SHORTCUT}"
  Delete "$SMPROGRAMS\$AppStartMenuFolder\${ADEL_BACKUPS_SHORTCUT}"
!macroend
