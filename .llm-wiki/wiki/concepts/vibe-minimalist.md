> Related: [[design-principles]]

# Vibe: Minimalist / Clean

**Source:** [taste-skill/minimalist-skill](https://github.com/Leonxlnx/taste-skill/tree/main/skills/minimalist-skill) — Premium Utilitarian Minimalism.

Notion, Linear, Superhuman. Functional, not decorative. Warm monochrome, typographic contrast, flat bento grids.

## Absolute Bans

- ❌ Inter, Roboto, Open Sans — use Geist, SF Pro, Switzer instead
- ❌ Lucide, Feather, Heroicons — use Phosphor (Bold), Radix Icons
- ❌ `shadow-md`/`shadow-lg` — shadow must be ultra-diffuse, opacity < 0.05
- ❌ Neon, gradients, glassmorphism
- ❌ `rounded-full` for large cards/containers
- ❌ Emoji anywhere
- ❌ AI copywriting: "Elevate", "Seamless", "Unleash", "Next-Gen"

## Typography

- **Body/UI**: SF Pro Display, Geist Sans, Switzer
- **Hero headings**: Lyon Text, Newsreader, Playfair Display — tracking -0.02em, leading 1.1
- **Mono**: Geist Mono, SF Mono, JetBrains Mono
- **Body text**: don't use `#000`, use `#111` or `#2F3437`. Line-height 1.6

## Color

| Role | Value |
|------|-------|
| Canvas | `#FFFFFF` / `#F7F6F3` |
| Card surface | `#FFFFFF` / `#F9F9F8` |
| Border | `#EAEAEA` or `rgba(0,0,0,0.06)` |
| Accent | Washed-out pastels: `#FDEBEC` (red), `#E1F3FE` (blue), `#EDF3EC` (green), `#FBF3DB` (yellow) |

## Cards

- `border: 1px solid #EAEAEA`, `border-radius: 8-12px`
- Padding: 24-40px
- No box-shadow

## Motion

- Entry: `translateY(12px)` + `opacity: 0` → `600ms`, `cubic-bezier(0.16, 1, 0.3, 1)`
- Hover: shadow shift from none to `0 2px 8px rgba(0,0,0,0.04)`
- Button active: `scale(0.98)`
- Stagger: `animation-delay: calc(var(--index) * 80ms)`
- Ambient: slow radial gradient blob, 20s+, opacity 0.02-0.04

## Reference

- linear.app, superhuman.com, raycast.com, notion.so

**Dials:** VARIANCE: 5 | MOTION: 3 | DENSITY: 2-3
