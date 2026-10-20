//! Spectral-gating noise reduction.
//!
//! The noise profile is estimated from the quietest frames of the signal,
//! then each STFT frame is attenuated per bin with an over-subtraction
//! Wiener-style mask. The mask is computed from power smoothed across
//! frequency and time: raw periodogram bins fluctuate several-fold around
//! the noise mean, which would otherwise let most noise through and cause
//! "musical noise". Gains also release slowly.

use crate::fft::{hann, Fft};

pub fn denoise_channel(x: &mut [f32], sample_rate: f32, strength: f32) {
    let frames = x.len();
    if frames == 0 {
        return;
    }
    let strength = strength.clamp(0.0, 1.0);
    let n = if sample_rate > 32000.0 { 2048 } else { 1024 };
    let hop = n / 4;
    let bins = n / 2 + 1;
    let fft = Fft::new(n);
    let win = hann(n);

    let noise_pow = estimate_noise(x, &fft, &win, hop);

    let over_sub = 1.0 + 2.5 * strength;
    let floor = 10f32.powf(-(6.0 + 24.0 * strength) / 20.0);
    let release = 0.55 + 0.3 * strength;

    let pad = n as isize;
    let total = frames + 2 * n;
    let mut out = vec![0f32; total];
    let mut wsum = vec![0f32; total];
    let mut re = vec![0f32; n];
    let mut im = vec![0f32; n];
    let mut prev_gain = vec![1f32; bins];
    let mut gain = vec![0f32; bins];
    let mut pow = vec![0f32; bins];
    let mut smooth_pow = noise_pow.clone();

    let mut pos: isize = -(n as isize) + hop as isize;
    while pos < frames as isize {
        for i in 0..n {
            let src = pos + i as isize;
            re[i] = if src >= 0 && (src as usize) < frames {
                x[src as usize] * win[i]
            } else {
                0.0
            };
            im[i] = 0.0;
        }
        fft.forward(&mut re, &mut im);

        for k in 0..bins {
            pow[k] = re[k] * re[k] + im[k] * im[k];
        }
        for k in 0..bins {
            let lo = k.saturating_sub(2);
            let hi = (k + 2).min(bins - 1);
            let pf = pow[lo..=hi].iter().sum::<f32>() / (hi - lo + 1) as f32;
            // Rise quickly so onsets survive, fall slowly so noise spikes don't open the gate.
            let alpha = if pf > smooth_pow[k] { 0.3 } else { 0.6 };
            smooth_pow[k] = alpha * smooth_pow[k] + (1.0 - alpha) * pf;
            let g = (1.0 - over_sub * noise_pow[k] / (smooth_pow[k] + 1e-12))
                .max(0.0)
                .sqrt()
                .max(floor);
            gain[k] = if g >= prev_gain[k] {
                g
            } else {
                prev_gain[k] * release + g * (1.0 - release)
            };
        }
        prev_gain.copy_from_slice(&gain);

        for k in 0..bins {
            let g = gain[k];
            re[k] *= g;
            im[k] *= g;
            if k > 0 && k < n / 2 {
                re[n - k] *= g;
                im[n - k] *= g;
            }
        }
        fft.inverse(&mut re, &mut im);

        for i in 0..n {
            let dst = (pos + pad + i as isize) as usize;
            out[dst] += re[i] * win[i];
            wsum[dst] += win[i] * win[i];
        }
        pos += hop as isize;
    }

    for i in 0..frames {
        let j = i + n;
        x[i] = if wsum[j] > 1e-6 { out[j] / wsum[j] } else { 0.0 };
    }
}

fn estimate_noise(x: &[f32], fft: &Fft, win: &[f32], hop: usize) -> Vec<f32> {
    let n = fft.len();
    let bins = n / 2 + 1;
    let frames = x.len();

    let mut positions: Vec<(f32, usize)> = Vec::new();
    let mut p = 0;
    loop {
        let end = (p + n).min(frames);
        let e: f32 = x[p..end].iter().map(|v| v * v).sum::<f32>() / (end - p).max(1) as f32;
        positions.push((e, p));
        if p + n >= frames {
            break;
        }
        p += hop;
    }
    positions.sort_by(|a, b| a.0.total_cmp(&b.0));
    let take = (positions.len() / 10).clamp(1, 400);

    let mut noise = vec![0f32; bins];
    let mut re = vec![0f32; n];
    let mut im = vec![0f32; n];
    for &(_, start) in positions.iter().take(take) {
        for i in 0..n {
            let s = start + i;
            re[i] = if s < frames { x[s] * win[i] } else { 0.0 };
            im[i] = 0.0;
        }
        fft.forward(&mut re, &mut im);
        for k in 0..bins {
            noise[k] += re[k] * re[k] + im[k] * im[k];
        }
    }
    for v in noise.iter_mut() {
        *v /= take as f32;
    }
    noise
}

#[cfg(test)]
mod tests {
    use super::*;

    fn lcg(seed: &mut u32) -> f32 {
        *seed = seed.wrapping_mul(1664525).wrapping_add(1013904223);
        (*seed >> 8) as f32 / (1u32 << 24) as f32 * 2.0 - 1.0
    }

    #[test]
    fn reduces_noise_in_silent_parts() {
        let sr = 48000.0;
        let len = 48000 * 2;
        let mut seed = 1;
        let mut x: Vec<f32> = (0..len)
            .map(|i| {
                let noise = lcg(&mut seed) * 0.05;
                let tone = if i > len / 2 {
                    (2.0 * std::f32::consts::PI * 440.0 * i as f32 / sr).sin() * 0.5
                } else {
                    0.0
                };
                noise + tone
            })
            .collect();
        let rms = |s: &[f32]| (s.iter().map(|v| v * v).sum::<f32>() / s.len() as f32).sqrt();
        let before_noise = rms(&x[4800..len / 2 - 4800]);
        let before_tone = rms(&x[len / 2 + 4800..len - 4800]);
        denoise_channel(&mut x, sr, 0.8);
        let after_noise = rms(&x[4800..len / 2 - 4800]);
        let after_tone = rms(&x[len / 2 + 4800..len - 4800]);
        assert!(after_noise < before_noise * 0.12, "noise {before_noise} -> {after_noise}");
        assert!(after_tone > before_tone * 0.8, "tone {before_tone} -> {after_tone}");
    }
}
