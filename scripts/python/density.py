import math

import matplotlib.pyplot as plt
import numpy as np

W = H = 20
MEAN_DENSITY = 0.20
STD_DENSITY = 0.20
BROAD_LEN = 20  # larger = smoother
LOCAL_LEN = 2.5  # smaller = more local variation
BROAD_WEIGHT = 0.6
LOCAL_WEIGHT = 0.8

_SEED = np.random.randint(0, 2**32)  # 42
_SEED2 = _SEED ^ 0xA5A5A5A5


def splitmix64(n: int) -> int:
    n = (n + 0x9E3779B97F4A7C15) & 0xFFFFFFFFFFFFFFFF
    n ^= n >> 30
    n = (n * 0xBF58476D1CE4E5B9) & 0xFFFFFFFFFFFFFFFF
    n ^= n >> 27
    n = (n * 0x94D049BB133111EB) & 0xFFFFFFFFFFFFFFFF
    n ^= n >> 31
    return n & 0xFFFFFFFFFFFFFFFF


def grad_angle(xi: int, yi: int, seed: int) -> float:
    # Map hashed lattice point → angle in [0, 2π)
    h = splitmix64(
        (xi * 0x1F123BB5 + yi * 0x9E3779B1 + seed * 0x94D049BB) & 0xFFFFFFFFFFFFFFFF
    )
    return (h / 2**64) * (2.0 * math.pi)


def fade(t: float) -> float:
    # Quintic smoothstep 6t^5 - 15t^4 + 10t^3
    return t * t * t * (t * (t * 6 - 15) + 10)


def grad_dot(xi: int, yi: int, x: float, y: float, seed: int) -> float:
    a = grad_angle(xi, yi, seed)
    gx, gy = math.cos(a), math.sin(a)
    return gx * (x - xi) + gy * (y - yi)


def gradient_noise_2d(x: float, y: float, length_scale: float, seed: int) -> float:
    """
    2D gradient (Perlin-style) noise:
      - gradients from stateless hash
      - quintic fade interpolation
      - approx output in [-1,1], mean ~0
    """
    s = length_scale
    xf = x / s
    yf = y / s

    x0 = math.floor(xf)
    y0 = math.floor(yf)
    x1 = x0 + 1
    y1 = y0 + 1
    tx = xf - x0
    ty = yf - y0
    u = fade(tx)
    v = fade(ty)

    n00 = grad_dot(x0, y0, xf, yf, seed)
    n10 = grad_dot(x1, y0, xf, yf, seed)
    n01 = grad_dot(x0, y1, xf, yf, seed)
    n11 = grad_dot(x1, y1, xf, yf, seed)

    nx0 = n00 + u * (n10 - n00)
    nx1 = n01 + u * (n11 - n01)
    return nx0 + v * (nx1 - nx0)


# Keep combined amplitude stable
_norm = math.sqrt(BROAD_WEIGHT**2 + LOCAL_WEIGHT**2) + 1e-12


def get_mine_density(x: int, y: int) -> float:
    broad = gradient_noise_2d(float(x), float(y), BROAD_LEN, _SEED)
    local = gradient_noise_2d(float(x), float(y), LOCAL_LEN, _SEED2)
    z = (BROAD_WEIGHT * broad + LOCAL_WEIGHT * local) / _norm  # ~zero-mean, unit-ish std
    d = MEAN_DENSITY + STD_DENSITY * z
    # Light clamp for safety
    return 0.0 if d < 0.0 else (1.0 if d > 1.0 else d)


densities = np.zeros((H, W), dtype=np.float32)
for y in range(H):
    for x in range(W):
        cx = x - W // 2
        cy = y - H // 2
        densities[y, x] = get_mine_density(cx, cy)

mean_pct = densities.mean() * 100
std_pct = densities.std() * 100

dx = densities[:, 1:] - densities[:, :-1]
dy = densities[1:, :] - densities[:-1, :]
adj_std = np.concatenate([dx.ravel(), dy.ravel()]).std() * 100

print(f"Mean: {mean_pct:.2f}%  Std: {std_pct:.2f}%")
print(f"Adjacent diff std: {adj_std:.3f}pp")

fig, (ax1, ax2) = plt.subplots(2, 1, figsize=(8, 10))
im = ax1.imshow(
    densities * 100, origin="lower", extent=[-W // 2, W // 2 - 1, -H // 2, H // 2 - 1]
)
ax1.set_title(f"Mine Density ({W}×{H}), Centered at (0,0)")
ax1.set_xlabel("x (centered)")
ax1.set_ylabel("y (centered)")
plt.colorbar(im, ax=ax1, label="Density (%)")

ax2.hist((densities * 100).ravel(), bins=40)
ax2.set_title("Density Distribution")
ax2.set_xlabel("Density (%)")
plt.tight_layout()
plt.savefig("density_map.png")
