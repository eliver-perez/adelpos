# Contesta el diálogo nativo «Guardar reporte» de la aplicación instalada (etapa 9E, prueba de humo): espera la
# ventana, escribe la ruta del archivo y pulsa Guardar. Playwright no puede tocar un diálogo nativo; este script
# hace lo que haría la persona. Solo actúa sobre una ventana con ese título.
#
# En Windows 11 el cuadro «Nombre de archivo» y el botón «Guardar» del diálogo común se exponen a UI
# Automation como paneles sin patrones de valor ni de invocación, así que se usan sus ventanas nativas: el
# campo de texto (clase Edit) dentro del cuadro y el mensaje BM_CLICK del botón.
param(
    [Parameter(Mandatory = $true)][string]$Path,
    [int]$TimeoutSeconds = 40
)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName UIAutomationClient, UIAutomationTypes
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class AdelDialog {
    [DllImport("user32.dll", CharSet = CharSet.Unicode)]
    public static extern IntPtr FindWindowEx(IntPtr parent, IntPtr after, string className, string title);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)]
    public static extern IntPtr SendMessage(IntPtr hwnd, uint message, IntPtr wParam, string lParam);
    [DllImport("user32.dll")]
    public static extern IntPtr SendMessage(IntPtr hwnd, uint message, IntPtr wParam, IntPtr lParam);
    // Busca en profundidad la primera ventana de una clase dentro de otra.
    public static IntPtr FindDescendant(IntPtr parent, string className) {
        IntPtr child = IntPtr.Zero;
        while ((child = FindWindowEx(parent, child, null, null)) != IntPtr.Zero) {
            var name = new System.Text.StringBuilder(64);
            GetClassName(child, name, 64);
            if (name.ToString() == className) { return child; }
            IntPtr found = FindDescendant(child, className);
            if (found != IntPtr.Zero) { return found; }
        }
        return IntPtr.Zero;
    }
    [DllImport("user32.dll", CharSet = CharSet.Unicode)]
    static extern int GetClassName(IntPtr hwnd, System.Text.StringBuilder name, int size);
}
'@
$WM_SETTEXT = 0x000C
$BM_CLICK = 0x00F5
$automation = [System.Windows.Automation.AutomationElement]
$scope = [System.Windows.Automation.TreeScope]::Descendants
function Condition($property, $value) {
    New-Object System.Windows.Automation.PropertyCondition $property, $value
}

$root = $automation::RootElement
$deadline = (Get-Date).AddSeconds($TimeoutSeconds)
$dialog = $null
while (-not $dialog -and (Get-Date) -lt $deadline) {
    # El diálogo es hijo de la ventana de la aplicación: se busca en todo el árbol.
    $dialog = $root.FindFirst($scope, (Condition $automation::NameProperty 'Guardar reporte'))
    if (-not $dialog) {
        Start-Sleep -Milliseconds 300
    }
}
if (-not $dialog) {
    throw 'No apareció el diálogo «Guardar reporte».'
}
$nameBox = $dialog.FindFirst($scope, (Condition $automation::AutomationIdProperty 'FileNameControlHost'))
if (-not $nameBox) {
    throw 'No se encontró el cuadro del nombre del archivo.'
}
$edit = [AdelDialog]::FindDescendant([IntPtr]$nameBox.Current.NativeWindowHandle, 'Edit')
if ($edit -eq [IntPtr]::Zero) {
    throw 'No se encontró el campo de texto del nombre del archivo.'
}
[AdelDialog]::SendMessage($edit, $WM_SETTEXT, [IntPtr]::Zero, $Path) | Out-Null
Start-Sleep -Milliseconds 300
# El botón Guardar: identificador 1 con ese nombre (un archivo de la carpeta también puede tener el
# identificador 1 en la lista).
$buttons = $dialog.FindAll($scope, (Condition $automation::AutomationIdProperty '1'))
$save = $buttons | Where-Object { $_.Current.Name -eq 'Guardar' -and $_.Current.NativeWindowHandle -ne 0 } | Select-Object -First 1
if (-not $save) {
    throw 'No se encontró el botón Guardar.'
}
[AdelDialog]::SendMessage([IntPtr]$save.Current.NativeWindowHandle, $BM_CLICK, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null
Write-Output "Guardado como $Path"
