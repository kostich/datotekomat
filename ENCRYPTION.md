# JSDAT-1 Encrypted Block - Format Specification (Edition 1)

This document describes the **exact** byte layout of the encrypted block stored
in the JSDAT-1 data area when the FSEntry type is `TYPE_ENCRYPTED = 0x04`. It is
intended for multiple implementations, so the format is **frozen**: any future
change must bump the `ver` byte and provide a migration path.

## Byte layout

| Field | Size | Description |
|-------|------|-------------|
| `magic` | 4 B | Encrypted-file marker: UTF-8 Cyrillic `Ш` + `Ф` only (4 bytes: `0xD0 0xA8 0xD0 0xA4`; no quote characters) |
| `ver` | 1 B | Format edition (currently `0x01`) |
| `salt` | 16 B | Random value (salt) for scrypt |
| `nonce` | 24 B | Unique number (XChaCha20 nonce) |
| `payload` | N + 16 B | Ciphertext (N bytes) + Poly1305 authenticator (16 B) |

The header is exactly **45 B** (`magic` + `ver` + `salt` + `nonce`). The minimum
block size is **61 B** (header + 16 B AEAD tag even for empty plaintext).

`FSEntry.Size` for a `TYPE_ENCRYPTED` entry is the **length of the entire block**
(header + payload), not the plaintext length. `FSEntry.Checksum` is the
**CRC32 of the entire block**, never of the plaintext.

```
offset  0      4      5     21     45        45+N        45+N+16
        +------+------+-----+------+----------+-----------+
        |magic | ver  |salt | nonce|  cipher  |  tag (16) |
        +------+------+-----+------+----------+-----------+
        |<--- 45 B header --->|<------ payload N+16 ----->|
```

## Key derivation (scrypt)

```text
key (32 B) = scrypt(passphrase, salt, N=32768, r=8, p=1, dkLen=32)
```

## Authenticated encryption (XChaCha20-Poly1305)

```text
payload = XChaCha20-Poly1305.Seal(
    key   = key,
    nonce = nonce,            // 24 B
    aad   = header[0:45],     // magic || ver || salt || nonce
    msg   = plaintext,
)
```

**AAD** covers the entire header; any change to `magic`, `ver`, `salt`, or
`nonce` will be detected on `Open`.

Decryption is the inverse:

```text
plaintext = XChaCha20-Poly1305.Open(
    key   = scrypt(passphrase, blob[5:21], 32768, 8, 1, 32),
    nonce = blob[21:45],
    aad   = blob[0:45],
    ct    = blob[45:],         // includes 16 B tag
)
```

## Placement in the data area

The block is stored **contiguously** in the FAT chain. The number of sectors is
`ceil(len(blob) / BytesPerSector)`. The last sector is **explicitly zero-padded**
(see `WriteBlob` in `sfat/dataentry.go`); the AEAD tag has no tolerance for
trailing bytes.

The minimum `BytesPerSector` for `кшу`/`кшс` is **512 B** (see
`MinBytesPerSectorForEncryption`). Devices with a smaller sector size (e.g.
`-бпс 16`, the default in tests) reject encrypted entries.

## Golden vector (for independent implementation)

```text
plaintext  = "Здраво!"                          (13 B UTF-8)
passphrase = "тест"
salt       = 01 02 03 04 05 06 07 08 09 0A 0B 0C 0D 0E 0F 10
nonce      = 10 11 12 13 14 15 16 17 18 19 1A 1B 1C 1D 1E 1F
             20 21 22 23 24 25 26 27

blob (74 B):
  D0 A8 D0 A4                                       // magic: Ш, Ф (4 B)
  01                                                // ver
  01 02 03 04 05 06 07 08 09 0A 0B 0C 0D 0E 0F 10   // salt
  10 11 12 13 14 15 16 17 18 19 1A 1B 1C 1D 1E 1F
  20 21 22 23 24 25 26 27                           // nonce
  D4 A8 6C F8 0C DA B9 00 13 9A AE B1 3C           // ciphertext (13 B)
  3D D3 65 CA 34 42 CB 87 13 FF 39 34 84 8D 1C DD  // Poly1305 tag (16 B)
```

`Seal` appends the tag immediately after the ciphertext (offsets 45–57 and 58–73).

The test `TestEncblob_GoldenVector` in `sfat/encblob_test.go` keeps this vector
as a regression guard. If it changes, existing encrypted files can no longer be
opened and `ver` must be bumped.

## What is not protected (threat model)

| Field | Visible in the clear | Rationale |
|-------|----------------------|-----------|
| Filename | yes | Stored in the FSEntry in the clear |
| On-disk size | yes | `FSEntry.Size` is the block length |
| Timestamps | yes | On the FSEntry, not in the block |
| Permissions (rwx) | yes | On the FSEntry |
| File existence | yes | Directory structure is in the clear |
| Order of encrypted sectors | yes | FAT chain is in the clear |
| **File content** | **no** | XChaCha20-Poly1305 |
| Header integrity | protected by Poly1305 | AAD = header |
| Block integrity | protected by CRC32 + Poly1305 | CRC gives early corruption warning |

This model matches selective per-file encryption (e.g. eCryptfs), not
whole-disk encryption (e.g. LUKS / VeraCrypt).

---

# JSDAT-1 шифровани блок - спецификација формата (издање 1)

Овај документ описује **тачан** распоред бајтова шифрованог блока који се
складишти у податковној области JSDAT-1 када је тип СД ставке
`TYPE_ENCRYPTED = 0x04`. Намењен је за вишеструке израде, стога је
формат **замрзнут** и било која будућа измена мора подићи `ver` бајт и
обезбедити пут миграције.

## Распоред бајтова

| Поље | Величина | Опис |
|------|----------|------|
| `magic` | 4 B | Маркер шифроване датотеке: UTF-8 слова `Ш` + `Ф` (4 бајта: `0xD0 0xA8 0xD0 0xA4`; без наводника) |
| `ver` | 1 B | Издање формата (тренутно `0x01`) |
| `salt` | 16 B | Насумична вредност (со) за scrypt |
| `nonce` | 24 B | Јединствени број (XChaCha20 нонс) |
| `payload` | N + 16 B | Шифровани подаци (N бајтова) + Poly1305 аутентикатор (16 B) |

Заглавље је тачно **45 B** (`magic` + `ver` + `salt` + `nonce`). Минимални
блок је **61 B** (заглавље + 16 B AEAD ознака чак и за празан отворени текст).

`FSEntry.Size` за ставку типа `TYPE_ENCRYPTED` је **дужина читавог блока**
(заглавље + payload), не дужина отвореног текста. `FSEntry.Checksum` је
**CRC32 целокупног блока**, никада отвореног текста.

```
offset  0      4      5     21     45        45+N        45+N+16
        +------+------+-----+------+----------+-----------+
        |magic | ver  |salt | nonce|  cipher  |  tag (16) |
        +------+------+-----+------+----------+-----------+
        |<--- 45 B header --->|<------ payload N+16 ----->|
```

## Деривација кључа (scrypt)

```text
key (32 B) = scrypt(passphrase, salt, N=32768, r=8, p=1, dkLen=32)
```

## Аутентификовано шифровање (XChaCha20-Poly1305)

```text
payload = XChaCha20-Poly1305.Seal(
    key   = key,
    nonce = nonce,            // 24 B
    aad   = header[0:45],     // magic || ver || salt || nonce
    msg   = plaintext,
)
```

**AAD** покрива читаво заглавље; било која измена `magic`, `ver`, `salt`
или `nonce` биће откривена при `Open`.

Дешифровање је инверзно:

```text
plaintext = XChaCha20-Poly1305.Open(
    key   = scrypt(passphrase, blob[5:21], 32768, 8, 1, 32),
    nonce = blob[21:45],
    aad   = blob[0:45],
    ct    = blob[45:],         // includes 16 B tag
)
```

## Размештај у податковну област

Блок се чува **узастопно** у ТДД ланцу. Број сектора је
`ceil(len(blob) / BytesPerSector)`. Последњи сектор се **експлицитно
попуњава нулама** (видети `WriteBlob` у `sfat/dataentry.go`), AEAD ознака
нема толеранцију за заостале бајтове.

Минимум `BytesPerSector` за `кшу`/`кшс` је **512 B** (видети
`MinBytesPerSectorForEncryption`). Уређаји са мањим сектором (нпр.
`-бпс 16` који је подразумевани у тестовима) одбијају шифроване ставке.

## Златни вектор (за независну имплементацију)

```text
plaintext  = "Здраво!"                          (13 B UTF-8)
passphrase = "тест"
salt       = 01 02 03 04 05 06 07 08 09 0A 0B 0C 0D 0E 0F 10
nonce      = 10 11 12 13 14 15 16 17 18 19 1A 1B 1C 1D 1E 1F
             20 21 22 23 24 25 26 27

blob (74 B):
  D0 A8 D0 A4                                       // magic: Ш, Ф (4 B)
  01                                                // ver
  01 02 03 04 05 06 07 08 09 0A 0B 0C 0D 0E 0F 10   // salt
  10 11 12 13 14 15 16 17 18 19 1A 1B 1C 1D 1E 1F
  20 21 22 23 24 25 26 27                           // nonce
  D4 A8 6C F8 0C DA B9 00 13 9A AE B1 3C           // ciphertext (13 B)
  3D D3 65 CA 34 42 CB 87 13 FF 39 34 84 8D 1C DD  // Poly1305 tag (16 B)
```

`Seal` додаје ознаку непосредно иза **шифрованог текста** (помераји 45–57 и 58–73).

Тест `TestEncblob_GoldenVector` у `sfat/encblob_test.go` чува овај вектор 
као регресиону одбрану, ако се промени, **ниједна** постојећа шифрована датотека 
не може више бити отворена и `ver` мора бити подигнут.

## Шта није заштићено (модел претње)

| Поље | Видљиво у отвореном | Образложење |
|------|---------------------|-------------|
| Назив датотеке | да | СД ставка чува назив у отвореном |
| Величина на диску | да | `FSEntry.Size` је дужина блока |
| Временске ознаке | да | На СД ставци, не у блоку |
| Овлашћења (rwx) | да | На СД ставци |
| Постојање датотеке | да | Структура фасцикле је у отвореном |
| Поредак шифрованих сектора | да | ТДД ланац је у отвореном |
| **Садржај датотеке** | **не** | XChaCha20-Poly1305 |
| Целовитост заглавља | штити Poly1305 | AAD = заглавље |
| Целовитост блока | штити CRC32 + Poly1305 | CRC рано упозорава на оштећење |

Овај модел одговара селективном шифровању на нивоу датотеке (нпр. eCryptfs)
а не шифровању целог диска (нпр. LUKS / VeraCrypt).
