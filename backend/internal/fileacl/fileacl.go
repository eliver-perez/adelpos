// Package fileacl protege archivos y carpetas privados del servidor (etapa 9E): secretos, claves TLS,
// configuración con la contraseña de la base y la carpeta de datos de la instalación de Windows.
//
// En macOS los permisos 0600/0700 con que se crean ya los protegen y Protect no hace nada. En Windows esos
// bits solo influyen en el atributo de solo lectura: la protección real es la lista de control de acceso
// (ACL). Protect fija una ACL propia, sin herencia de la carpeta superior, que concede control total
// únicamente a la cuenta que ejecuta el servidor, a SYSTEM y al grupo Administradores (el equivalente de
// root en macOS, que también puede leer un archivo 0600).
package fileacl
