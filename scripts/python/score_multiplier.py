import numpy as np
import matplotlib.pyplot as plt
import math

dLo = 10.0
dHi = 50.0
yLo = 0.2
yHi = 10.0
q   = 1.1453726926484111  # precomputed so that f(21%) == 1.0

def smoothstep5(t: float) -> float:
    if t <= 0.0: return 0.0
    if t >= 1.0: return 1.0
    t2 = t * t
    t3 = t2 * t
    return t3 * (10.0 - 15.0 * t + 6.0 * t2)  # 6t^5 - 15t^4 + 10t^3

def score_multiplier_go_style(density: float) -> float:
    d = float(density)
    if d <= dLo: return yLo
    if d >= dHi: return yHi
    t = (d - dLo) / (dHi - dLo)
    s = smoothstep5(math.pow(t, q))
    return yLo + (yHi - yLo) * s

# Reference curve via exact inversion (for validation)
def S(t: float) -> float:
    # same as smoothstep5 but without guards (we clamp in caller)
    return t**3 * (10 - 15*t + 6*t*t)

def inv_S(y: float, tol=1e-12, max_iter=80) -> float:
    lo, hi = 0.0, 1.0
    for _ in range(max_iter):
        mid = 0.5 * (lo + hi)
        f = S(mid) - y
        if abs(f) < tol or hi - lo < tol:
            return mid
        if f > 0: hi = mid
        else:     lo = mid
    return 0.5 * (lo + hi)

D_LO, D_MED, D_HI = dLo, 21.0, dHi
Y_LO, Y_MED, Y_HI = yLo, 1.0,  yHi
alpha = (Y_MED - Y_LO) / (Y_HI - Y_LO)
t21   = (D_MED - D_LO) / (D_HI - D_LO)
u     = inv_S(alpha)
q_ref = math.log(u) / math.log(t21)  # should match the precomputed q closely

def score_multiplier_reference(density: float) -> float:
    d = float(density)
    if d <= D_LO: return Y_LO
    if d >= D_HI: return Y_HI
    t = (d - D_LO) / (D_HI - D_LO)
    return Y_LO + (Y_HI - Y_LO) * S(t**q_ref)

# Generate and compare
densities = np.linspace(0, 60, 601)
y_go  = np.array([score_multiplier_go_style(d) for d in densities])
y_ref = np.array([score_multiplier_reference(d) for d in densities])
max_abs_diff = float(np.max(np.abs(y_go - y_ref)))

# Plot
fig, ax = plt.subplots(figsize=(8, 4.8))
ax.plot(densities, y_ref, label="Reference (inversion-derived)")
ax.axhline(1.0, linestyle=":", linewidth=1)

for d in [10, 21, 50]:
    y = score_multiplier_go_style(d)
    ax.scatter([d], [y], zorder=3)
    ax.annotate(f"{d}%\n{y:.2f}×", (d, y), textcoords="offset points", xytext=(0,6), ha="center", fontsize=8)

ax.set_xlabel("Chunk mine density (%)")
ax.set_ylabel("Score multiplier")
ax.set_title(f"Multiplier vs density (max |Δ| ≈ {max_abs_diff:.3e})")
ax.set_ylim(0, 11)
ax.grid(True, alpha=0.3)
ax.legend(loc="upper left")

out_path = "density_multiplier_go_equiv.png"
plt.tight_layout()
plt.savefig(out_path, dpi=160)
out_path, q, q_ref, max_abs_diff
