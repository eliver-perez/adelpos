# Origen de los archivos del contrato de licencias

Los archivos de `contracts/licencias/v1/` y los dos documentos de `docs/licencias/` que lista la tabla son **copias
exactas, de solo lectura**, del repositorio del servidor de licencias. **No se editan, no se formatean y no se
regeneran** en AD-EL POS: si algo no coincide con la integración, el error está en la integración o se propone una
versión 1.1 del contrato. Las rutas copiadas están en `.prettierignore` y `.editorconfig` para que ninguna herramienta
las toque.

- **Repositorio**: `https://github.com/eliver-perez/adelpos-license-server`
- **Etiqueta**: `v1.0.0` (objeto de la etiqueta `2a9275511196cd9dc6213da955c1459a13c72f51`)
- **Commit**: `309dcff220430669c6167579d51145b4c8b8cbd5` (17/11/2026, «Etapa 5, Fase 8: cierre de la etapa y versión
  1.0.0»)
- **Copiado**: 23/11/2026, en la Fase 0 de la etapa L1.
- **Origen de cada archivo**: `contracts/v1/…` del repositorio pasa a `contracts/licencias/v1/…`, y
  `docs/CONTRATO_LICENCIAS_ADEL_POS.md` y `docs/INTEGRACION_ADEL_POS.md` pasan a `docs/licencias/` con el mismo
  nombre.
- **Contrato**: versión 1.0 definitiva, SHA-256 `e3435f16e27e11c85ac263ef2accfa72aad07cf30dbcdee0a0b8b22326314058`.

El enlace de `contracts/licencias/v1/README.md` al contrato (`../../docs/CONTRATO_LICENCIAS_ADEL_POS.md`) es el del
repositorio de origen; en AD-EL POS el contrato está en
[`docs/licencias/CONTRATO_LICENCIAS_ADEL_POS.md`](../../docs/licencias/CONTRATO_LICENCIAS_ADEL_POS.md).

Para comprobar las copias:

```sh
sed -n 's/^| `\([^`]*\)` *| *[0-9]* *| `\([0-9a-f]\{64\}\)` *|$/\2  \1/p' contracts/licencias/ORIGEN.md | shasum -a 256 -c
```

El verificador de referencia se ejecuta igual que en el repositorio de origen (§2 de la guía):

```sh
env GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go run -C contracts/licencias/v1/verify . ../fixtures/vectors.json
```

## SHA-256 de cada archivo copiado

| Archivo                                                                        | Bytes  | SHA-256                                                            |
| ------------------------------------------------------------------------------ | ------ | ------------------------------------------------------------------ |
| `contracts/licencias/v1/README.md`                                             | 10588  | `e6d754575ab181f54d8be4fd48873e35938d7505a509765a71c24356b5b17361` |
| `contracts/licencias/v1/fixtures/licenses/closing-deactivated.lic`             | 1297   | `a209b887bbd638dc0800b4b9f9f804e41151ace2ff50f51b52646ff802ed4a69` |
| `contracts/licencias/v1/fixtures/licenses/closing-revoked.lic`                 | 1269   | `134045617ac0ea1c69341778ea7e95b05f9d83baad3bbd9082452fac03fa49c0` |
| `contracts/licencias/v1/fixtures/licenses/closing-transferred.lic`             | 1253   | `60c0793ce1bc97d8b832cc41173e38ec7344ab0371acf8205abac07e6c599d3f` |
| `contracts/licencias/v1/fixtures/licenses/custom.lic`                          | 1274   | `7b6d4706f5a9cd9043bd1874bd5c1781b0ca6000f80f8c2d38ae05ad43699d06` |
| `contracts/licencias/v1/fixtures/licenses/perpetual-0-months.lic`              | 1240   | `6ca52903f2cbba46da127d5d35863be2dc731f0d248f3b339b31c3896877ecf2` |
| `contracts/licencias/v1/fixtures/licenses/perpetual-12-months.lic`             | 1236   | `2492e5af9f4d11e2c16011cd39eed7b6fecec8bec6417b378c8e4d00358b1662` |
| `contracts/licencias/v1/fixtures/licenses/perpetual-6-months.lic`              | 1240   | `8b4917e4823b93798340e32b75d83e58a059d1afbe240cf8d0a29e365fecbe16` |
| `contracts/licencias/v1/fixtures/licenses/perpetual-with-maintenance.lic`      | 1240   | `c303a860aa16d66e22bc06f988c42f92c77fe111f68ad188571dacabaef29e7b` |
| `contracts/licencias/v1/fixtures/licenses/plan-upgrade.lic`                    | 1280   | `634f2d9ae8c2508e43b409153adece5c302a20e32456dee470333b5e37a667eb` |
| `contracts/licencias/v1/fixtures/licenses/renewed-subscription.lic`            | 1282   | `a3ae362b9df3d615f9f8e47bb6b6d0c40aa05df18a819b3a53cbe83fc3fed65e` |
| `contracts/licencias/v1/fixtures/licenses/rotated-key.lic`                     | 1236   | `f8323fd656df1f18c8ea89f7dd1afcef324fc73d7bbb7d9a0d71c12b2b4dbb1d` |
| `contracts/licencias/v1/fixtures/licenses/scheduled-reduction-applied.lic`     | 1277   | `e0d6167a4177e3c8080112efce6a807d077938f869fa6cdc71d6a2b775ebb5fe` |
| `contracts/licencias/v1/fixtures/licenses/self-service-transfer-before.lic`    | 1284   | `706da9b00ba9b8299cc4c3e89ef31867e5e2ee454ee4bc53163daf68bab45986` |
| `contracts/licencias/v1/fixtures/licenses/self-service-transfer-closing.lic`   | 1297   | `8ccc48c8ecee4a16eac357c1db15ace5859f671d2612e8ae99d3cf8b6483dad2` |
| `contracts/licencias/v1/fixtures/licenses/self-service-transfer-new.lic`       | 1284   | `44ea093442f323ca6a31fca6137e8c45cca8c8a76ad7eeb6f42a52c7b7a29448` |
| `contracts/licencias/v1/fixtures/licenses/subscription-annual.lic`             | 1282   | `6ced9d535bd5b828ef3a28f8e63e6387bcc673b28b347dfbb30c27f4a4d4ba34` |
| `contracts/licencias/v1/fixtures/licenses/subscription-monthly.lic`            | 1284   | `b8cd9be3b6282a23bcad3da81ff9585dc490df6aa30278a171ceb9293fe5700b` |
| `contracts/licencias/v1/fixtures/licenses/trial-15-days.lic`                   | 1261   | `64a4cf50526ccbb3998678f401fb1e6867a102f783d4d0737d93f7f06dda1a66` |
| `contracts/licencias/v1/fixtures/licenses/trial-converted-to-perpetual.lic`    | 1240   | `ad35d847e7dad6f1a236c436bbd5e066cad239ff88f6bd4eafd4930b03466cbb` |
| `contracts/licencias/v1/fixtures/licenses/trial-converted-to-subscription.lic` | 1284   | `9a689c7782578191b53e90d0d675653b7dac35dfc4b4f29f9b48463040d01cbf` |
| `contracts/licencias/v1/fixtures/requests/activate.licreq`                     | 567    | `b037f9d0a676b5666733f7e650883df823a5da3f77c0fc1ee3701928d7a3d607` |
| `contracts/licencias/v1/fixtures/requests/deactivate.licreq`                   | 666    | `ae237126009e95d64a8e0d15a7e34c9561ebcbaccc4b087b19af74fe51f70f45` |
| `contracts/licencias/v1/fixtures/requests/renew.licreq`                        | 659    | `22a238c0c10fb0c3f4a65e993f259f4f20c6162daef2f898dae4d5a90d6e012a` |
| `contracts/licencias/v1/fixtures/vectors.json`                                 | 175749 | `0319ec3f3b95d9306df6ca3869b29f0053088715b96a50bfb53ff0beabc02c63` |
| `contracts/licencias/v1/generate-fixtures.php`                                 | 50539  | `8c1c05ed1d1434a64e2647834112ff3d04b38a565ab10ade4b6fef8acc00dc47` |
| `contracts/licencias/v1/license.schema.json`                                   | 7069   | `8fe3013bd67cbd017e073d008d45afa537532bafefbfaa5994064abe9373988a` |
| `contracts/licencias/v1/openapi.yaml`                                          | 29053  | `778f69e8f8ec41c99a3b26d59666ca1ba09d037eef94409a27bc186e6ce0a72b` |
| `contracts/licencias/v1/verify/go.mod`                                         | 47     | `9192fc6d8814e29d5ad0627093e89a3e29b02e627b651bd07797f14ead6b33a0` |
| `contracts/licencias/v1/verify/license.go`                                     | 15396  | `1a7cd3791995e35349da087acf248629dc626eab913c15cbf5e88125b0b88b43` |
| `contracts/licencias/v1/verify/main.go`                                        | 13112  | `56d8e8043668fa896a2aa19493c74975a356a10b8e2937e9442902cff10b7305` |
| `contracts/licencias/v1/verify/protocol.go`                                    | 3680   | `e98f0d0b5f790640105436dd554533cc079445e8535d9c745bdc49c015a13e69` |
| `contracts/licencias/v1/verify/state.go`                                       | 1729   | `314df784bc3308e8225eed7214fdce8e96fdda3ded6dbf88ca1a3def8e3cbbd2` |
| `docs/licencias/CONTRATO_LICENCIAS_ADEL_POS.md`                                | 42754  | `e3435f16e27e11c85ac263ef2accfa72aad07cf30dbcdee0a0b8b22326314058` |
| `docs/licencias/INTEGRACION_ADEL_POS.md`                                       | 14599  | `15a19069e1c834ef735e9607158c740b7fe5f1287ee388f4250d8be5736672e8` |
