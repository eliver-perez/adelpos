# APK de prueba (A1-D8): R8 quita el código y los recursos sin uso, pero no ofusca los nombres, para que los errores
# y los registros sean legibles. Las bibliotecas (kotlinx.serialization, OkHttp, Room, Compose) traen sus reglas.
-dontobfuscate
