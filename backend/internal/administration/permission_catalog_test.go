package administration

import (
	"slices"
	"strings"
	"testing"
)

// Catálogo de permisos de la etapa A3 (§3, A3-D2 a A3-D6): completo, cada permiso en un solo grupo, con descripción,
// dependencias válidas y sin ciclos, capacidades y conjuntos de fábrica cerrados bajo las dependencias.

func catalogEntriesByKey(t *testing.T) map[string]catalogEntry {
	t.Helper()
	entries := map[string]catalogEntry{}
	for _, entry := range permissionCatalogEntries {
		if _, repeated := entries[entry.key]; repeated {
			t.Fatalf("%s está dos veces en el catálogo", entry.key)
		}
		entries[entry.key] = entry
	}
	return entries
}

func TestPermissionCatalogCoversEveryRegisteredPermission(t *testing.T) {
	entries := catalogEntriesByKey(t)
	if len(Permissions) < 150 {
		t.Fatalf("solo hay %d permisos registrados", len(Permissions))
	}
	if len(entries) != len(Permissions) {
		t.Fatalf("el catálogo tiene %d permisos y hay %d registrados", len(entries), len(Permissions))
	}
	groups := map[string]bool{}
	for _, group := range permissionGroups {
		groups[group.code] = true
	}
	if len(groups) != 10 {
		t.Fatalf("se esperaban los diez grupos de §3.2: %v", groups)
	}
	for _, permission := range Permissions {
		entry, found := entries[permission.Key]
		if !found {
			t.Fatalf("%s no está en el catálogo", permission.Key)
		}
		if !groups[entry.group] {
			t.Fatalf("%s tiene un grupo desconocido: %q", permission.Key, entry.group)
		}
		if len(entry.description) < 15 || !strings.HasSuffix(entry.description, ".") {
			t.Fatalf("%s necesita una descripción corta, en una frase: %q", permission.Key, entry.description)
		}
		if strings.TrimSpace(permission.Description) == "" && entry.name == "" {
			t.Fatalf("%s no tiene nombre visible", permission.Key)
		}
	}
	catalog := buildPermissionCatalog()
	if len(catalog.Permissions) != len(Permissions) || len(catalog.Groups) != 10 {
		t.Fatalf("catálogo: %d permisos y %d grupos", len(catalog.Permissions), len(catalog.Groups))
	}
	total := 0
	for _, group := range catalog.Groups {
		if len(group.Permissions) == 0 {
			t.Fatalf("el grupo %s está vacío", group.Name)
		}
		total += len(group.Permissions)
	}
	if total != len(Permissions) {
		t.Fatalf("los grupos suman %d permisos y hay %d", total, len(Permissions))
	}
	// Nombres visibles en lenguaje del restaurante (§1 de la A3): sin la sigla de la pantalla de cocina.
	for _, permission := range catalog.Permissions {
		if strings.Contains(permission.Name, "KDS") || strings.Contains(permission.Description, "KDS") {
			t.Fatalf("%s usa la sigla KDS: %q", permission.Key, permission.Name)
		}
		if permission.Requires == nil {
			t.Fatalf("%s debe informar requires aunque esté vacío", permission.Key)
		}
	}
}

func TestPermissionDependenciesAreKnownAndAcyclic(t *testing.T) {
	entries := catalogEntriesByKey(t)
	for _, entry := range permissionCatalogEntries {
		for _, need := range entry.requires {
			if _, known := entries[need]; !known {
				t.Fatalf("%s necesita un permiso desconocido: %s", entry.key, need)
			}
			if need == entry.key {
				t.Fatalf("%s se necesita a sí mismo", entry.key)
			}
		}
	}
	// Recorrido en profundidad: un permiso que vuelve a aparecer en su propio camino es un ciclo.
	state := map[string]int{}
	var visit func(key string, path []string)
	visit = func(key string, path []string) {
		switch state[key] {
		case 1:
			t.Fatalf("ciclo de dependencias: %v → %s", path, key)
		case 2:
			return
		}
		state[key] = 1
		for _, need := range entries[key].requires {
			visit(need, append(path, key))
		}
		state[key] = 2
	}
	for _, entry := range permissionCatalogEntries {
		visit(entry.key, nil)
	}
	// Algunas que pide la especificación y la guía (§3.2 y docs/ANDROID_INSTALAR.md §4).
	for key, need := range map[string]string{
		"accounts.open":     "accounts.read",
		"orders.cancel":     "orders.edit",
		"orders.confirm":    "orders.capture",
		"takeaway.manage":   "takeaway.read",
		"cash.shifts.close": "cash.counts.perform",
		"roles.manage":      "permissions.manage",
	} {
		if !slices.Contains(entries[key].requires, need) {
			t.Fatalf("%s debía necesitar %s", key, need)
		}
	}
}

func TestFactoryRolesAreClosedUnderDependencies(t *testing.T) {
	expectedSizes := map[string]int{"waiter": 14, "cashier": 36, "kitchen": 8, "supervisor": 89, "inventory": 20, "finance": 28}
	if len(factoryRoleCodes) != len(expectedSizes) || len(factoryRolePermissions) != len(expectedSizes) {
		t.Fatalf("roles de fábrica: %v", factoryRoleCodes)
	}
	for _, code := range factoryRoleCodes {
		keys, found := factoryPermissions(code)
		if !found {
			t.Fatalf("%s no tiene conjunto de fábrica", code)
		}
		if len(keys) != expectedSizes[code] {
			t.Fatalf("%s: %d permisos, se esperaban %d (anexo de la Fase 0)", code, len(keys), expectedSizes[code])
		}
		for index, key := range keys {
			if !knownPermission(key) {
				t.Fatalf("%s tiene un permiso desconocido: %s", code, key)
			}
			if index > 0 && keys[index-1] == key {
				t.Fatalf("%s repite %s", code, key)
			}
		}
		if closed := closePermissions(keys); !slices.Equal(closed, keys) {
			t.Fatalf("%s no está cerrado bajo sus dependencias; le faltan %v", code, missingFrom(closed, keys))
		}
	}
	for _, code := range []string{"administrator", "support", ""} {
		if _, found := factoryPermissions(code); found {
			t.Fatalf("%q no debía tener conjunto de fábrica", code)
		}
	}
}

// El Mesero de fábrica es todo lo de la guía (WaiterPermissions) salvo «Modificar» y «Cancelar unidades» (§3.1), así
// que una sesión con PIN conserva el conjunto completo.
func TestFactoryWaiterIsWaiterPermissionsWithoutEditAndCancel(t *testing.T) {
	waiter, _ := factoryPermissions("waiter")
	expected := []string{}
	for _, key := range WaiterPermissions {
		if key != "orders.edit" && key != "orders.cancel" {
			expected = append(expected, key)
		}
	}
	slices.Sort(expected)
	if !slices.Equal(waiter, expected) {
		t.Fatalf("Mesero de fábrica: %v\nse esperaba %v", waiter, expected)
	}
	if scoped := waiterScope(waiter); len(scoped) != len(waiter) {
		t.Fatalf("una sesión con PIN perdería permisos del Mesero de fábrica: %v", scoped)
	}
}

func TestPermissionCapabilitiesUseKnownPermissions(t *testing.T) {
	codes := map[string]bool{}
	for _, capability := range permissionCapabilities {
		if codes[capability.Code] {
			t.Fatalf("capacidad repetida: %s", capability.Code)
		}
		codes[capability.Code] = true
		if capability.Name == "" || len(capability.Description) < 15 || len(capability.Permissions) == 0 {
			t.Fatalf("capacidad incompleta: %+v", capability)
		}
		for _, key := range capability.Permissions {
			if !knownPermission(key) {
				t.Fatalf("%s tiene un permiso desconocido: %s", capability.Code, key)
			}
		}
	}
	// Las nueve de §3.2 y las dos de A3-D5.
	for _, code := range []string{"tables", "takeaway", "collect", "cash", "kitchen", "reports", "catalog", "staff", "system", "inventory", "authorize"} {
		if !codes[code] {
			t.Fatalf("falta la capacidad %s", code)
		}
	}
	// Un rol de fábrica armado con sus capacidades tiene cada una completa (no parcial) si las elige todas.
	cook, _ := factoryPermissions("kitchen")
	for _, capability := range permissionCapabilities {
		if capability.Code == "kitchen" && len(missingFrom(capability.Permissions, cook)) != 0 {
			t.Fatalf("la capacidad Cocina debía estar completa en el rol Cocina: %v", missingFrom(capability.Permissions, cook))
		}
	}
}
