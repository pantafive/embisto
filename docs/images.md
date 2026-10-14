# Image variants

Pictures in previews point at other sites. Those URLs cause several problems:

- They expire. Instagram's and Facebook's image links are signed for hours.
- Some sites refuse hotlinking.
- Every client that loads a picture reveals its address to that site.
- A phone downloads a 1280-pixel original to show an 80-pixel thumbnail.
- An `http` picture breaks on an `https` page.

So embisto serves image variants: resized, re-encoded copies of a card's pictures, from its own address at `/i/`, with immutable cache headers.

## How it works

A variant is served only at a URL the server signed. `/i/<signature>/<size>/<source>` names:

- **source:** the original picture's URL, in unpadded base64url;
- **size:** a preset's name, or an explicit `WxH-fit` or `WxH-fill`;
- **signature:** an HMAC-SHA256 of `size/source` under the key `EMBISTO_IMAGE_KEY`, in unpadded base64url.

The signature is checked before anything else in the URL is read. A URL changed anywhere is refused with 403 `invalid_signature`.

When variants are on and `EMBISTO_PUBLIC_URL` is set, cards carry the signed URLs as `variants` of their `image`, `icon` and `images`. Each variant has its preset and its URL. It also has its size when the original's size is known, so a client can pick the smallest variant that fills its view:

```json
"image": {
  "url": "https://i.ytimg.com/vi/dQw4w9WgXcQ/maxresdefault.jpg", "width": 1280, "height": 720,
  "variants": [
    {"preset": "thumb", "url": "https://embisto.example.com/i/…/thumb/…", "webp_url": "https://embisto.example.com/i/…/thumb.webp/…", "width": 160, "height": 160},
    {"preset": "card", "url": "https://embisto.example.com/i/…/card/…", "webp_url": "https://embisto.example.com/i/…/card.webp/…", "width": 600, "height": 338},
    {"preset": "large", "url": "https://embisto.example.com/i/…/large/…", "webp_url": "https://embisto.example.com/i/…/large.webp/…", "width": 1200, "height": 675}
  ]
}
```

A preset that comes out the same size as one before it is left out. That happens to every preset after the first for a small icon. SVG and ICO sources get no variants, because `/i/` refuses them.

`embisto sign <size> <url>` prints a variant's URL, for a client backend that signs its own URLs or for a test.

## Sizes

| Preset | Box |
|---|---|
| `thumb` | 160×160, filled: scaled to cover the box and cropped around the center |
| `card` | 600 wide, fitted: scaled down to fit, keeping the proportions |
| `large` | 1200 wide, fitted |

`EMBISTO_IMAGE_PRESETS` replaces the presets, written as `name=WxH-mode` separated by commas, such as `thumb=160x160-fill,card=600x0-fit`. A zero side does not bound.

An explicit size is accepted only when both sides come from 0, 64, 96, 128, 160, 240, 320, 480, 600, 640, 800, 960 and 1200, so that the variants of one picture, and with them the cache, stay few. Pictures are never enlarged.

## Processing

Variants are made in pure Go, without cgo. Each step bounds the next:

1. **Slot.** A picture waits for a free processing slot: four at a time, for up to two seconds. While it waits it holds no file in memory. A picture that gets no slot is refused with 503 `busy` and `Retry-After`.
2. **Fetch.** The source is fetched through the same client as pages, which refuses addresses that are not public. The limits are 10 MiB and 10 seconds; a larger file is refused with 413 `image_too_large`.
3. **Header.** The format and size are read from the header, never from `Content-Type`. Only JPEG, PNG, GIF and WebP pass; anything else gets 415 `unsupported_image`. A picture over 16 megapixels is refused before it is decoded, with 413 `too_many_pixels`.
4. **Memory.** From the header alone, embisto works out what decoding and scaling the picture will take: the decoded pixels, an intermediate, the scaler's own buffer and the output. Every picture being resized shares 256 MiB; a picture waits up to two seconds for room, else gets 503 `busy`, and one that could never fit gets 413 `too_many_pixels`.
5. **Decode and scale.** The picture is decoded with the standard library and `golang.org/x/image` (the first frame of a GIF), then scaled with Catmull-Rom from `x/image/draw`. A picture more than twice the output's size is first shrunk to twice it with a fast bilinear pass, since Catmull-Rom holds the output's width by its source's height in 32-byte pixels. A picture taller than three times its width keeps its top, one wider than three times its height its middle, and no output is larger than a megapixel.
6. **Encode.** The result is PNG when it has transparency, JPEG at quality 82 otherwise. A size ending in `.webp`, such as `card.webp`, asks for WebP instead. Upstream bytes are never passed through.

## Formats

Each variant offers two URLs for one size:

- **`url`** serves JPEG, or PNG for a picture with transparency. Every client decodes these.
- **`webp_url`** serves lossy WebP at quality 75, about half the bytes.

The format is part of the signed size segment. A cache therefore never holds two formats under one URL and needs no `Vary: Accept`, and URLs signed before WebP existed keep serving JPEG and PNG. Changing the suffix of a signed URL breaks its signature (403).

WebP is encoded by [`github.com/gen2brain/vpx`](https://github.com/gen2brain/vpx) v0.2.1: a pure Go port of libwebp, byte-exact against it, with SIMD on amd64 and arm64. It adds about 0.7 MB to the binary.

**Transparency** stays in WebP's alpha plane, which is lossless while the colour is lossy. On three transparent PNG files scaled to 600 pixels wide, lossy WebP was far smaller than either lossless option:

| Picture | PNG | Lossless WebP | Lossy WebP with alpha |
|---|---|---|---|
| Wikipedia logo, 600×548 | 241 KB | 157 KB | 32 KB |
| PNG transparency demo (Wikimedia), 600×450 | 169 KB | 173 KB | 64 KB |
| GitHub fluidicon, 512×512 | 45 KB | 33 KB | 17 KB |

Lossy WebP is about 60 ms slower to encode at 600 pixels. Lossless WebP is no smaller than PNG for photographs and only modestly smaller for drawings, so transparency goes lossy in WebP and stays PNG for clients without WebP.

At the 16-megapixel limit, a 1200-pixel WebP takes about 0.5 s against 0.44 s for JPEG, with the same peak memory, so the memory figure below holds.

A picture at the pixel limit takes about half a second to resize. The pictures being resized take at most 256 MiB between them, and the source files being read at most four times 10 MiB, so making variants holds about 300 MiB at worst; [configuration](configuration.md#memory) adds the caches to it. A 601×26622 PNG of under 100 KB, which once took 611 MiB, takes some 110 MiB as it resizes, within the 111 MiB it is admitted for. Ordinary preview pictures, such as a 1200×630 JPEG, take about 20 ms and a few megabytes.

## Caching

A picture's URL names its content for good, so a 200 carries `Cache-Control: public, max-age=31536000, immutable`, `X-Content-Type-Options: nosniff` and an `ETag` that is the URL's signature. A matching `If-None-Match` gets 304 without the source being fetched.

Every failure carries `Cache-Control: no-store`, so a passing upstream fault does not stay in a cache for a year.

## Keys

`EMBISTO_IMAGE_KEY` must have at least 32 bytes, for example `openssl rand -hex 32`; a shorter one stops the server at startup. Without a key variants are off: `/i/` is not served and cards carry none.

Rotating the key invalidates every URL signed with the old one. Cards in the preview cache keep the old URLs for as long as they are cached, up to `EMBISTO_CACHE_MAX_TTL`, so those URLs answer 403 until the cards are made afresh or purged.

## Limits of the approach

embisto fetches a source only when one of its variants is first requested; the response carries immutable cache headers, so caches keep the variant after that. A source that expires before anyone opens a variant of it, such as an Instagram link a card was made from hours earlier, leaves its variants nothing to fetch, so their URLs fail.

## Evaluation: WebP and AVIF output

These are the WebP and AVIF encoders that would keep the binary free of cgo, measured on 2026-10-13. WebP through `gen2brain/vpx` has since been adopted (see Formats above); AVIF has not.

| Package | What it is | License | Version, activity |
|---|---|---|---|
| `github.com/gen2brain/webp` | libwebp compiled to WebAssembly and transpiled to Go with wasm2go; it loads a system libwebp through purego when one is installed, unless built with `-tags nodynamic` | MIT | v0.6.4, 63 stars, active |
| `github.com/gen2brain/avif` | libavif with aom and dav1d compiled to WebAssembly, run in the process by the wazero runtime; also loads a system library unless built with `-tags nodynamic` | MIT | v0.6.0, 83 stars, active |
| `github.com/gen2brain/vpx/webp` | a pure Go port of libwebp, byte-exact against it, with SIMD on amd64 and arm64 | BSD-3-Clause | v0.2.1, 10 stars, young |
| `github.com/gen2brain/gav1d/avif` | a pure Go AV1 and AVIF encoder and decoder | BSD-2-Clause | v0.2.5, 7 stars, young |

All four build with `CGO_ENABLED=0` and contain no `import "C"`. Each was asked to encode three real preview pictures, each scaled to a width of 600 and of 1200:

- YouTube's 1280×720 JPEG thumbnail;
- GitHub's 1200×630 RGBA PNG;
- a Guardian 1200×630 JPEG.

The settings were JPEG quality 82, WebP quality 75 (method 4) and AVIF quality 60 (speed 8). Each codec ran in a process of its own on an Apple M-series laptop. Times are per picture after a first call; sizes are the three pictures' outputs.

| Codec | 600 px: time | 600 px: bytes | 1200 px: time | 1200 px: bytes | Peak RSS at 1200 | Binary growth |
|---|---|---|---|---|---|---|
| JPEG (stdlib) | 3 ms | 25–28 KB | 10–12 ms | 63–77 KB | 70 MB | — |
| WebP, wasm2go | 52–59 ms | 10–13 KB | 198–227 ms | 22–34 KB | 174 MB | +2.6 MB |
| WebP, pure Go (vpx) | 11–14 ms | 11–15 KB | 37–47 ms | 23–39 KB | 110 MB | +0.7 MB |
| AVIF, wazero | 148–163 ms (first call 0.9 s) | 11–14 KB | 441–489 ms (first call 1.3 s) | 22–41 KB | 965 MB | +4.8 MB |
| AVIF, pure Go (gav1d) | 33–35 ms | 13–19 KB | 113–117 ms | 25–41 KB | 74 MB | +1.6 MB |

Encoders at nominal qualities are not visually equal, so the byte counts compare sizes, not efficiency.

All four handled eight goroutines encoding at once without errors.

Findings:

- **Size.** WebP and AVIF cut the bytes of a JPEG at the same width by roughly half. At 1200 px, AVIF did no better than WebP on the GitHub PNG.
- **wasm2go WebP** is 15–20 times slower than JPEG. For a picture that is encoded once and then served from caches, that is affordable.
- **wazero AVIF** is the outlier: about a second of compilation on the first call, half a second per 1200-pixel picture, and close to a gigabyte of resident memory. It does not fit next to the preview pipeline in one small process.
- **The pure Go ports** are 3–5 times faster than the WebAssembly builds and far lighter. They are also new, with few users.

AVIF can wait until `gav1d` matures.
