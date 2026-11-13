//! Menú nativo de la aplicación (etapa 9C, fase C).
//!
//! El menú se arma en Rust, en español y sin depender de macOS: el submenú «AD-EL POS» (Acerca de,
//! Ocultar y Salir) solo existe en macOS, donde el primer submenú se convierte en el menú de la
//! aplicación; en Windows el menú empieza en Archivo. Edición conserva los elementos predefinidos
//! del sistema (deshacer, rehacer, cortar, copiar, pegar y seleccionar todo), sin los cuales los
//! campos pierden ⌘C/⌘V. No hay «Cerrar ventana» (⌘W cerraría la única ventana).
//!
//! Las opciones propias son una **lista cerrada** (`MENU_ACTIONS`): Rust no ejecuta ninguna acción
//! de negocio ni conoce rutas del servidor. Al elegir una opción, Rust envía `{ "action": id }` a
//! React por un `Channel` (operación cerrada `menu_subscribe`); React la ejecuta con el mismo
//! registro de acciones que la paleta y los atajos, es decir, con la misma función que el botón de
//! la interfaz. React informa qué opciones están habilitadas según los permisos y el contexto con
//! otra operación cerrada (`menu_set_state`). La única opción que atiende Rust es «Salir»: cierra
//! esta aplicación sin detener Go ni PostgreSQL, que funcionan sin ventana. Desde la etapa 9F
//! `menu_set_state` recibe además, opcional, el nombre visible de la precuenta (clave de una lista
//! cerrada) y cambia solo el texto de «Imprimir precuenta»; no abre rutas nuevas.
//!
//! El puente de pruebas de Playwright (`desktop/e2e/native-driver.ts`) y una prueba de Vitest leen
//! `MENU_ACTIONS` de este archivo con expresiones regulares: cada entrada conserva la forma
//! `MenuAction { id: "…", label: "…", mac: …, other: … }`.

use serde::Serialize;
use std::collections::{HashMap, HashSet};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Mutex;
use tauri::ipc::Channel;
use tauri::menu::{Menu, MenuEvent, MenuItem, PredefinedMenuItem, Submenu};
use tauri::{AppHandle, Manager, State, Wry};

/// Opción propia del menú: identificador de la acción de React, texto y acelerador por plataforma
/// (sintaxis de Tauri: `CmdOrCtrl` es ⌘ en macOS y Ctrl en Windows).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct MenuAction {
    pub id: &'static str,
    pub label: &'static str,
    pub mac: Option<&'static str>,
    pub other: Option<&'static str>,
}

/// Lista cerrada de opciones que ejecuta React. Los aceleradores son los mismos atajos del
/// registro de acciones de React (`desktop/src/commands/actions.ts`).
pub const MENU_ACTIONS: &[MenuAction] = &[
    MenuAction {
        id: "file.table_account",
        label: "Nueva cuenta en mesa",
        mac: Some("CmdOrCtrl+N"),
        other: Some("CmdOrCtrl+N"),
    },
    MenuAction {
        id: "file.account_without_table",
        label: "Cuenta sin mesa",
        mac: None,
        other: None,
    },
    MenuAction {
        id: "file.takeaway_order",
        label: "Nuevo pedido para llevar",
        mac: None,
        other: None,
    },
    MenuAction {
        id: "file.print_precheck",
        label: "Imprimir precuenta",
        mac: Some("CmdOrCtrl+P"),
        other: Some("CmdOrCtrl+P"),
    },
    MenuAction {
        id: "file.logout",
        label: "Cerrar sesión",
        mac: None,
        other: None,
    },
    MenuAction {
        id: "edit.find",
        label: "Buscar en la pantalla",
        mac: Some("CmdOrCtrl+F"),
        other: Some("CmdOrCtrl+F"),
    },
    MenuAction {
        id: "view.tables",
        label: "Mesas",
        mac: Some("CmdOrCtrl+1"),
        other: Some("CmdOrCtrl+1"),
    },
    MenuAction {
        id: "view.takeaway",
        label: "Para llevar",
        mac: Some("CmdOrCtrl+2"),
        other: Some("CmdOrCtrl+2"),
    },
    MenuAction {
        id: "view.kitchen",
        label: "Cocina",
        mac: Some("CmdOrCtrl+3"),
        other: Some("CmdOrCtrl+3"),
    },
    MenuAction {
        id: "view.cash",
        label: "Caja",
        mac: Some("CmdOrCtrl+4"),
        other: Some("CmdOrCtrl+4"),
    },
    MenuAction {
        id: "view.administration",
        label: "Administración",
        mac: Some("CmdOrCtrl+5"),
        other: Some("CmdOrCtrl+5"),
    },
    MenuAction {
        id: "view.palette",
        label: "Paleta de comandos",
        mac: Some("CmdOrCtrl+K"),
        other: Some("CmdOrCtrl+K"),
    },
    MenuAction {
        id: "view.fullscreen",
        label: "Pantalla completa",
        mac: Some("Ctrl+Super+F"),
        other: Some("F11"),
    },
    MenuAction {
        id: "view.theme",
        label: "Tema claro u oscuro",
        mac: None,
        other: None,
    },
    MenuAction {
        id: "cash.income",
        label: "Ingreso",
        mac: None,
        other: None,
    },
    MenuAction {
        id: "cash.withdrawal",
        label: "Retiro",
        mac: None,
        other: None,
    },
    MenuAction {
        id: "cash.expense",
        label: "Gasto",
        mac: None,
        other: None,
    },
    MenuAction {
        id: "cash.partial",
        label: "Corte parcial",
        mac: None,
        other: None,
    },
    MenuAction {
        id: "cash.drawer",
        label: "Abrir cajón",
        mac: None,
        other: None,
    },
    MenuAction {
        id: "cash.opening",
        label: "Abrir turno",
        mac: None,
        other: None,
    },
    MenuAction {
        id: "cash.open",
        label: "Ir a Caja",
        mac: None,
        other: None,
    },
    MenuAction {
        id: "help.shortcuts",
        label: "Atajos de teclado",
        mac: None,
        other: None,
    },
    MenuAction {
        id: "help.about",
        label: "Acerca de AD-EL POS",
        mac: None,
        other: None,
    },
];

/// «Salir» del menú Archivo: la única opción que atiende Rust (no llega a React).
pub const QUIT_ID: &str = "file.quit";

/// Opciones habilitadas antes de que React informe el estado (antes de iniciar sesión): las que no
/// dependen de permisos ni de una cuenta.
const INITIALLY_ENABLED: &[&str] = &["view.fullscreen", "help.shortcuts", "help.about"];

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Platform {
    MacOs,
    Other,
}

impl Platform {
    #[cfg_attr(target_os = "windows", allow(dead_code))]
    pub fn current() -> Self {
        if cfg!(target_os = "macos") {
            Self::MacOs
        } else {
            Self::Other
        }
    }
}

impl MenuAction {
    pub fn accelerator(&self, platform: Platform) -> Option<&'static str> {
        match platform {
            Platform::MacOs => self.mac,
            Platform::Other => self.other,
        }
    }
}

/// Elementos predefinidos del sistema (sus textos se fijan en español).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Native {
    Undo,
    Redo,
    Cut,
    Copy,
    Paste,
    SelectAll,
    Hide,
    HideOthers,
    ShowAll,
    Quit,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Entry {
    /// Opción propia de React (identificador de `MENU_ACTIONS`).
    Action(&'static str),
    /// Elemento predefinido del sistema.
    Native(Native),
    /// «Salir» de Archivo (`QUIT_ID`).
    Quit,
    Separator,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Section {
    pub title: &'static str,
    pub entries: Vec<Entry>,
}

/// Estructura del menú por plataforma (§3.1 de la especificación). Es un dato puro: las pruebas
/// la revisan sin abrir una ventana.
pub fn layout(platform: Platform) -> Vec<Section> {
    use Entry::{Action, Separator};
    let mut sections = Vec::new();
    if platform == Platform::MacOs {
        sections.push(Section {
            title: "AD-EL POS",
            entries: vec![
                Action("help.about"),
                Separator,
                Entry::Native(Native::Hide),
                Entry::Native(Native::HideOthers),
                Entry::Native(Native::ShowAll),
                Separator,
                Entry::Native(Native::Quit),
            ],
        });
    }
    sections.push(Section {
        title: "Archivo",
        entries: vec![
            Action("file.table_account"),
            Action("file.account_without_table"),
            Action("file.takeaway_order"),
            Separator,
            Action("file.print_precheck"),
            Separator,
            Action("file.logout"),
            Separator,
            Entry::Quit,
        ],
    });
    sections.push(Section {
        title: "Edición",
        entries: vec![
            Entry::Native(Native::Undo),
            Entry::Native(Native::Redo),
            Separator,
            Entry::Native(Native::Cut),
            Entry::Native(Native::Copy),
            Entry::Native(Native::Paste),
            Entry::Native(Native::SelectAll),
            Separator,
            Action("edit.find"),
        ],
    });
    sections.push(Section {
        title: "Ver",
        entries: vec![
            Action("view.tables"),
            Action("view.takeaway"),
            Action("view.kitchen"),
            Action("view.cash"),
            Action("view.administration"),
            Separator,
            Action("view.palette"),
            Separator,
            Action("view.fullscreen"),
            Action("view.theme"),
        ],
    });
    sections.push(Section {
        title: "Caja",
        entries: vec![
            Action("cash.income"),
            Action("cash.withdrawal"),
            Action("cash.expense"),
            Action("cash.partial"),
            Action("cash.drawer"),
            Action("cash.opening"),
            Separator,
            Action("cash.open"),
        ],
    });
    sections.push(Section {
        title: "Ayuda",
        entries: vec![Action("help.shortcuts"), Action("help.about")],
    });
    sections
}

pub fn action_by_id(id: &str) -> Option<&'static MenuAction> {
    MENU_ACTIONS.iter().find(|action| action.id == id)
}

/// Qué hace Rust con el identificador de un evento del menú.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Route {
    /// Se envía a React.
    React(&'static str),
    /// Cierra la aplicación (Go y PostgreSQL siguen funcionando).
    Quit,
    /// Elemento predefinido o desconocido: no se envía nada.
    Ignore,
}

pub fn route(id: &str) -> Route {
    if id == QUIT_ID {
        return Route::Quit;
    }
    match action_by_id(id) {
        Some(action) => Route::React(action.id),
        None => Route::Ignore,
    }
}

/// Valida el estado que informa React: solo identificadores de la lista cerrada, sin repetirse
/// más allá del tamaño de la lista.
pub fn validate_enabled(enabled: &[String]) -> Result<HashSet<&'static str>, String> {
    if enabled.len() > MENU_ACTIONS.len() {
        return Err("Estado del menú no válido.".into());
    }
    let mut accepted = HashSet::new();
    for id in enabled {
        let action = action_by_id(id).ok_or("Opción de menú no reconocida.")?;
        accepted.insert(action.id);
    }
    Ok(accepted)
}

/// Texto de «Imprimir precuenta» según el nombre visible del documento previo al cobro (etapa 9F,
/// §4.5): la misma lista cerrada de Go (`payments.PrecheckNames`). Cualquier otra clave se rechaza.
pub fn precheck_menu_label(name: &str) -> Option<&'static str> {
    match name {
        "precheck" => Some("Imprimir precuenta"),
        "sales_ticket" => Some("Imprimir ticket de venta"),
        "bill" => Some("Imprimir cuenta"),
        _ => None,
    }
}

/// Mensaje que recibe React por el canal del menú.
#[derive(Debug, Clone, Serialize, PartialEq, Eq)]
pub struct MenuMessage {
    pub action: &'static str,
}

/// Estado del menú: las opciones propias (una acción puede aparecer en dos submenús, como «Acerca
/// de» en macOS) y el único suscriptor vigente de React.
#[derive(Default)]
pub struct MenuState {
    items: Mutex<HashMap<&'static str, Vec<MenuItem<Wry>>>>,
    subscriber: Mutex<Option<(u64, Channel<MenuMessage>)>>,
    generation: AtomicU64,
}

#[cfg_attr(target_os = "windows", allow(dead_code))]
impl MenuState {
    fn replace_items(&self, items: HashMap<&'static str, Vec<MenuItem<Wry>>>) {
        if let Ok(mut current) = self.items.lock() {
            *current = items;
        }
    }

    /// Una suscripción nueva sustituye a la anterior (una recarga de la vista o el doble montaje
    /// de React en desarrollo no duplican los eventos).
    fn subscribe(&self, channel: Channel<MenuMessage>) -> Result<u64, String> {
        let identifier = self.generation.fetch_add(1, Ordering::SeqCst) + 1;
        let mut subscriber = self
            .subscriber
            .lock()
            .map_err(|_| "No se pudo preparar el menú.")?;
        *subscriber = Some((identifier, channel));
        Ok(identifier)
    }

    fn emit(&self, action: &'static str) {
        let Ok(mut subscriber) = self.subscriber.lock() else {
            return;
        };
        let failed = match subscriber.as_ref() {
            Some((_, channel)) => channel.send(MenuMessage { action }).is_err(),
            None => false,
        };
        // Una vista que ya no existe deja de recibir eventos.
        if failed {
            *subscriber = None;
        }
    }
}

// Windows no adjunta el menú nativo (main.rs, etapa 9E): estas piezas solo se usan en macOS.
#[cfg_attr(target_os = "windows", allow(dead_code))]
fn native_item(handle: &AppHandle, kind: Native) -> tauri::Result<PredefinedMenuItem<Wry>> {
    match kind {
        Native::Undo => PredefinedMenuItem::undo(handle, Some("Deshacer")),
        Native::Redo => PredefinedMenuItem::redo(handle, Some("Rehacer")),
        Native::Cut => PredefinedMenuItem::cut(handle, Some("Cortar")),
        Native::Copy => PredefinedMenuItem::copy(handle, Some("Copiar")),
        Native::Paste => PredefinedMenuItem::paste(handle, Some("Pegar")),
        Native::SelectAll => PredefinedMenuItem::select_all(handle, Some("Seleccionar todo")),
        Native::Hide => PredefinedMenuItem::hide(handle, Some("Ocultar AD-EL POS")),
        Native::HideOthers => PredefinedMenuItem::hide_others(handle, Some("Ocultar otras")),
        Native::ShowAll => PredefinedMenuItem::show_all(handle, Some("Mostrar todas")),
        Native::Quit => PredefinedMenuItem::quit(handle, Some("Salir de AD-EL POS")),
    }
}

/// Arma el menú de la plataforma y guarda las opciones propias para habilitarlas desde React.
#[cfg_attr(target_os = "windows", allow(dead_code))]
pub fn build(handle: &AppHandle) -> tauri::Result<Menu<Wry>> {
    let platform = Platform::current();
    let menu = Menu::new(handle)?;
    let mut controlled: HashMap<&'static str, Vec<MenuItem<Wry>>> = HashMap::new();
    for section in layout(platform) {
        let submenu = Submenu::new(handle, section.title, true)?;
        for entry in section.entries {
            match entry {
                Entry::Action(id) => {
                    let Some(action) = action_by_id(id) else {
                        continue;
                    };
                    let item = MenuItem::with_id(
                        handle,
                        action.id,
                        action.label,
                        INITIALLY_ENABLED.contains(&action.id),
                        action.accelerator(platform),
                    )?;
                    submenu.append(&item)?;
                    controlled.entry(action.id).or_default().push(item);
                }
                Entry::Native(kind) => submenu.append(&native_item(handle, kind)?)?,
                Entry::Quit => {
                    let item = MenuItem::with_id(handle, QUIT_ID, "Salir", true, None::<&str>)?;
                    submenu.append(&item)?;
                }
                Entry::Separator => submenu.append(&PredefinedMenuItem::separator(handle)?)?,
            }
        }
        menu.append(&submenu)?;
    }
    if let Some(state) = handle.try_state::<MenuState>() {
        state.replace_items(controlled);
    }
    Ok(menu)
}

/// Eventos del menú: «Salir» cierra la aplicación; las opciones propias se envían a React.
#[cfg_attr(target_os = "windows", allow(dead_code))]
pub fn handle_event(app: &AppHandle, event: MenuEvent) {
    match route(event.id().as_ref()) {
        // Cerrar la ventana no detiene Go ni PostgreSQL: el servidor y las demás terminales
        // siguen funcionando.
        Route::Quit => app.exit(0),
        Route::React(action) => {
            if let Some(state) = app.try_state::<MenuState>() {
                state.emit(action);
            }
        }
        Route::Ignore => {}
    }
}

/// Operación cerrada: React se suscribe a los eventos del menú.
#[tauri::command]
pub fn menu_subscribe(
    state: State<'_, MenuState>,
    channel: Channel<MenuMessage>,
) -> Result<u64, String> {
    state.subscribe(channel)
}

/// Operación cerrada: React informa qué opciones propias están habilitadas (las demás se
/// deshabilitan) y, desde la etapa 9F, el nombre visible de la precuenta. Un identificador o un
/// nombre fuera de su lista cerrada rechaza todo el estado.
#[tauri::command]
pub fn menu_set_state(
    state: State<'_, MenuState>,
    enabled: Vec<String>,
    precheck_name: Option<String>,
) -> Result<(), String> {
    let accepted = validate_enabled(&enabled)?;
    let precheck_label = match precheck_name.as_deref() {
        Some(name) => {
            Some(precheck_menu_label(name).ok_or("Nombre de la precuenta no reconocido.")?)
        }
        None => None,
    };
    let items = state
        .items
        .lock()
        .map_err(|_| "No se pudo actualizar el menú.")?;
    for (id, entries) in items.iter() {
        let on = accepted.contains(id);
        for item in entries {
            item.set_enabled(on)
                .map_err(|_| "No se pudo actualizar el menú.")?;
            if *id == "file.print_precheck" {
                if let Some(label) = precheck_label {
                    item.set_text(label)
                        .map_err(|_| "No se pudo actualizar el menú.")?;
                }
            }
        }
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn action_ids(platform: Platform) -> Vec<&'static str> {
        layout(platform)
            .into_iter()
            .flat_map(|section| section.entries)
            .filter_map(|entry| match entry {
                Entry::Action(id) => Some(id),
                _ => None,
            })
            .collect()
    }

    fn titles(platform: Platform) -> Vec<&'static str> {
        layout(platform)
            .into_iter()
            .map(|section| section.title)
            .collect()
    }

    fn section(platform: Platform, title: &str) -> Section {
        layout(platform)
            .into_iter()
            .find(|section| section.title == title)
            .expect("submenú")
    }

    // Acelerador con la sintaxis que acepta Tauri (muda): modificadores conocidos y una tecla.
    // MenuItem::with_id descarta en silencio un acelerador inválido, por eso se revisa aquí.
    fn valid_accelerator(accelerator: &str) -> bool {
        let parts: Vec<&str> = accelerator.split('+').collect();
        let Some((key, modifiers)) = parts.split_last() else {
            return false;
        };
        let modifiers_ok = modifiers
            .iter()
            .all(|modifier| ["CmdOrCtrl", "Ctrl", "Super", "Shift", "Alt"].contains(modifier));
        let key_ok = (key.len() == 1 && key.chars().all(|c| c.is_ascii_alphanumeric()))
            || (key.starts_with('F')
                && key[1..].parse::<u8>().is_ok_and(|n| (1..=12).contains(&n)));
        modifiers_ok && key_ok
    }

    #[test]
    fn menu_follows_the_specification_on_both_platforms() {
        assert_eq!(
            titles(Platform::MacOs),
            ["AD-EL POS", "Archivo", "Edición", "Ver", "Caja", "Ayuda"]
        );
        // Windows: sin el submenú de la aplicación (no depende de comportamientos de macOS).
        assert_eq!(
            titles(Platform::Other),
            ["Archivo", "Edición", "Ver", "Caja", "Ayuda"]
        );
        for platform in [Platform::MacOs, Platform::Other] {
            let file = section(platform, "Archivo");
            assert_eq!(file.entries.last(), Some(&Entry::Quit));
            let labels: Vec<&str> = file
                .entries
                .iter()
                .filter_map(|entry| match entry {
                    Entry::Action(id) => action_by_id(id).map(|action| action.label),
                    _ => None,
                })
                .collect();
            assert_eq!(
                labels,
                [
                    "Nueva cuenta en mesa",
                    "Cuenta sin mesa",
                    "Nuevo pedido para llevar",
                    "Imprimir precuenta",
                    "Cerrar sesión"
                ]
            );
            let view: Vec<&str> = section(platform, "Ver")
                .entries
                .iter()
                .filter_map(|entry| match entry {
                    Entry::Action(id) => action_by_id(id).map(|action| action.label),
                    _ => None,
                })
                .collect();
            assert_eq!(
                view,
                [
                    "Mesas",
                    "Para llevar",
                    "Cocina",
                    "Caja",
                    "Administración",
                    "Paleta de comandos",
                    "Pantalla completa",
                    "Tema claro u oscuro"
                ]
            );
            let cash: Vec<&str> = section(platform, "Caja")
                .entries
                .iter()
                .filter_map(|entry| match entry {
                    Entry::Action(id) => action_by_id(id).map(|action| action.label),
                    _ => None,
                })
                .collect();
            assert_eq!(
                cash,
                [
                    "Ingreso",
                    "Retiro",
                    "Gasto",
                    "Corte parcial",
                    "Abrir cajón",
                    "Abrir turno",
                    "Ir a Caja"
                ]
            );
            assert_eq!(
                section(platform, "Ayuda").entries,
                [Entry::Action("help.shortcuts"), Entry::Action("help.about")]
            );
        }
        let application = section(Platform::MacOs, "AD-EL POS");
        assert_eq!(
            application.entries.first(),
            Some(&Entry::Action("help.about"))
        );
        assert!(application.entries.contains(&Entry::Native(Native::Hide)));
        assert_eq!(
            application.entries.last(),
            Some(&Entry::Native(Native::Quit))
        );
    }

    #[test]
    fn edit_menu_keeps_the_system_items_and_there_is_no_close_window() {
        for platform in [Platform::MacOs, Platform::Other] {
            let edit = section(platform, "Edición");
            for kind in [
                Native::Undo,
                Native::Redo,
                Native::Cut,
                Native::Copy,
                Native::Paste,
                Native::SelectAll,
            ] {
                assert!(edit.entries.contains(&Entry::Native(kind)), "{kind:?}");
            }
        }
        // No existe un elemento para cerrar la ventana (⌘W) ni un submenú Ventana.
        assert!(!titles(Platform::MacOs).contains(&"Ventana"));
        for action in MENU_ACTIONS {
            assert_ne!(action.accelerator(Platform::MacOs), Some("CmdOrCtrl+W"));
            assert_ne!(action.accelerator(Platform::Other), Some("CmdOrCtrl+W"));
        }
    }

    #[test]
    fn closed_list_is_complete_unique_and_valid() {
        let mut seen = HashSet::new();
        for action in MENU_ACTIONS {
            assert!(seen.insert(action.id), "repetido: {}", action.id);
            assert!(!action.label.is_empty());
            for platform in [Platform::MacOs, Platform::Other] {
                if let Some(accelerator) = action.accelerator(platform) {
                    assert!(valid_accelerator(accelerator), "{accelerator}");
                }
            }
        }
        assert!(!seen.contains(QUIT_ID));
        // Cada opción de la lista aparece en el menú y el menú no usa identificadores fuera de ella.
        for platform in [Platform::MacOs, Platform::Other] {
            let used: HashSet<&str> = action_ids(platform).into_iter().collect();
            assert_eq!(used, seen, "{platform:?}");
        }
        // Solo «Acerca de» se repite (menú de la aplicación y Ayuda), y solo en macOS.
        let mac = action_ids(Platform::MacOs);
        let repeated: Vec<&str> = mac
            .iter()
            .copied()
            .filter(|id| mac.iter().filter(|other| *other == id).count() > 1)
            .collect();
        assert_eq!(repeated, ["help.about", "help.about"]);
        let other = action_ids(Platform::Other);
        assert_eq!(other.len(), MENU_ACTIONS.len());
        // Ningún acelerador se repite dentro de una plataforma.
        for platform in [Platform::MacOs, Platform::Other] {
            let mut accelerators = HashSet::new();
            for action in MENU_ACTIONS {
                if let Some(accelerator) = action.accelerator(platform) {
                    assert!(accelerators.insert(accelerator), "{accelerator}");
                }
            }
        }
        for id in INITIALLY_ENABLED {
            assert!(action_by_id(id).is_some());
        }
    }

    #[test]
    fn shortcuts_use_command_on_macos_and_control_on_windows() {
        let tables = action_by_id("view.tables").unwrap();
        assert_eq!(tables.accelerator(Platform::MacOs), Some("CmdOrCtrl+1"));
        assert_eq!(tables.accelerator(Platform::Other), Some("CmdOrCtrl+1"));
        for (id, key) in [
            ("view.takeaway", "2"),
            ("view.kitchen", "3"),
            ("view.cash", "4"),
            ("view.palette", "K"),
            ("edit.find", "F"),
        ] {
            let action = action_by_id(id).unwrap();
            let expected = format!("CmdOrCtrl+{key}");
            assert_eq!(action.accelerator(Platform::MacOs), Some(expected.as_str()));
            assert_eq!(action.accelerator(Platform::Other), Some(expected.as_str()));
        }
        // Pantalla completa con el atajo estándar de cada sistema.
        let fullscreen = action_by_id("view.fullscreen").unwrap();
        assert_eq!(
            fullscreen.accelerator(Platform::MacOs),
            Some("Ctrl+Super+F")
        );
        assert_eq!(fullscreen.accelerator(Platform::Other), Some("F11"));
        // Las acciones de caja (financieras) no tienen acelerador: solo abren su hoja.
        for id in [
            "cash.income",
            "cash.withdrawal",
            "cash.expense",
            "cash.partial",
            "cash.drawer",
            "cash.opening",
        ] {
            assert_eq!(action_by_id(id).unwrap().mac, None);
            assert_eq!(action_by_id(id).unwrap().other, None);
        }
    }

    #[test]
    fn events_are_routed_through_the_closed_list() {
        assert_eq!(route("view.tables"), Route::React("view.tables"));
        assert_eq!(route("help.about"), Route::React("help.about"));
        assert_eq!(route(QUIT_ID), Route::Quit);
        // Elementos predefinidos (sus identificadores los genera el sistema) o desconocidos.
        assert_eq!(route("copy"), Route::Ignore);
        assert_eq!(route("restaurant_open_account"), Route::Ignore);
        assert_eq!(route(""), Route::Ignore);
        let message = serde_json::to_value(MenuMessage {
            action: "view.palette",
        })
        .unwrap();
        assert_eq!(message, serde_json::json!({"action": "view.palette"}));
    }

    #[test]
    fn precheck_label_follows_the_closed_list() {
        assert_eq!(precheck_menu_label("precheck"), Some("Imprimir precuenta"));
        assert_eq!(
            precheck_menu_label("sales_ticket"),
            Some("Imprimir ticket de venta")
        );
        assert_eq!(precheck_menu_label("bill"), Some("Imprimir cuenta"));
        assert_eq!(precheck_menu_label("Imprimir precuenta"), None);
        assert_eq!(precheck_menu_label(""), None);
        // La opción conserva su texto predeterminado en la lista cerrada (lo comparan Vitest y
        // Playwright).
        assert_eq!(
            action_by_id("file.print_precheck").unwrap().label,
            "Imprimir precuenta"
        );
    }

    #[test]
    fn menu_state_accepts_only_the_closed_list() {
        let accepted = validate_enabled(&["view.tables".into(), "cash.income".into()]).unwrap();
        assert_eq!(accepted.len(), 2);
        assert!(accepted.contains("cash.income"));
        assert!(validate_enabled(&[]).unwrap().is_empty());
        assert_eq!(
            validate_enabled(&["view.tables".into(), "api_request".into()]).unwrap_err(),
            "Opción de menú no reconocida."
        );
        assert_eq!(
            validate_enabled(&[QUIT_ID.into()]).unwrap_err(),
            "Opción de menú no reconocida."
        );
        let too_many = vec!["view.tables".to_string(); MENU_ACTIONS.len() + 1];
        assert_eq!(
            validate_enabled(&too_many).unwrap_err(),
            "Estado del menú no válido."
        );
    }
}
