//! WSOLA time-stretching and pitch-shifting.
//!
//! All channels share the same segment offsets (searched on the mono mix)
//! so the stereo image stays phase-coherent.

use crate::fft::hann;

const DECIMATE: usize = 4;

/// Stretch planar `input` (`channels * frames`) so its length becomes
/// `frames * len_factor`. Returns a planar buffer of `out_frames` per channel.
pub fn wsola(
    input: &[f32],
    frames: usize,
    channels: usize,
    sample_rate: f32,
    len_factor: f64,
    out_frames: usize,
) -> Vec<f32> {
    let mut out = vec![0f32; channels * out_frames];
    if frames == 0 || out_frames == 0 {
        return out;
    }

    let w = (((sample_rate * 0.040) as usize).max(64)) & !1;
    let hs = w / 2;
    let ha = hs as f64 / len_factor;
    let tol = ((sample_rate * 0.012) as usize).max(16);
    let win = hann(w);

    let mono: Vec<f32> = (0..frames)
        .map(|i| (0..channels).map(|c| input[c * frames + i]).sum::<f32>() / channels as f32)
        .collect();
    let dec: Vec<f32> = mono
        .chunks(DECIMATE)
        .map(|c| c.iter().sum::<f32>() / c.len() as f32)
        .collect();

    let span = out_frames + w;
    let mut acc = vec![0f32; channels * span];
    let mut wsum = vec![0f32; span];
    let mut prev_in: usize = 0;
    let mut k: usize = 0;

    loop {
        let out_pos = k * hs;
        if out_pos >= out_frames {
            break;
        }
        let nominal = (k as f64 * ha).round() as usize;
        let in_pos = if k == 0 {
            0
        } else {
            let target = prev_in + hs;
            best_offset(&mono, &dec, target, nominal, tol, w, frames)
        };

        for c in 0..channels {
            let src = &input[c * frames..(c + 1) * frames];
            let dst = &mut acc[c * span..(c + 1) * span];
            for i in 0..w {
                let s = in_pos + i;
                if s < frames {
                    dst[out_pos + i] += src[s] * win[i];
                }
            }
        }
        for i in 0..w {
            wsum[out_pos + i] += win[i];
        }
        prev_in = in_pos;
        k += 1;
    }

    for c in 0..channels {
        for i in 0..out_frames {
            let ws = wsum[i];
            out[c * out_frames + i] = if ws > 1e-6 { acc[c * span + i] / ws } else { 0.0 };
        }
    }
    out
}

fn best_offset(
    mono: &[f32],
    dec: &[f32],
    target: usize,
    nominal: usize,
    tol: usize,
    w: usize,
    frames: usize,
) -> usize {
    let max_start = frames.saturating_sub(1);
    let lo = nominal.saturating_sub(tol).min(max_start);
    let hi = (nominal + tol).min(max_start);
    if target >= frames {
        return nominal.min(max_start);
    }

    let corr_len = (w / 2 / DECIMATE).max(8);
    let td = target / DECIMATE;
    let mut best = nominal.min(max_start);
    let mut best_score = f32::MIN;
    let mut cand = lo - lo % DECIMATE;
    while cand <= hi {
        let cd = cand / DECIMATE;
        let score = correlate(dec, td, cd, corr_len);
        if score > best_score {
            best_score = score;
            best = cand;
        }
        cand += DECIMATE;
    }

    let fine_len = (w / 2).max(16);
    let flo = best.saturating_sub(DECIMATE).max(lo);
    let fhi = (best + DECIMATE).min(hi);
    let mut fine_best = best;
    let mut fine_score = f32::MIN;
    for cand in flo..=fhi {
        let score = correlate(mono, target, cand, fine_len);
        if score > fine_score {
            fine_score = score;
            fine_best = cand;
        }
    }
    fine_best
}

fn correlate(x: &[f32], a: usize, b: usize, len: usize) -> f32 {
    let n = len.min(x.len().saturating_sub(a)).min(x.len().saturating_sub(b));
    let mut dot = 0f32;
    let mut energy = 1e-9f32;
    for i in 0..n {
        let bv = x[b + i];
        dot += x[a + i] * bv;
        energy += bv * bv;
    }
    dot / energy.sqrt()
}

/// Cubic Hermite resampling of a planar buffer to `out_frames` per channel.
pub fn resample(input: &[f32], frames: usize, channels: usize, out_frames: usize) -> Vec<f32> {
    let mut out = vec![0f32; channels * out_frames];
    if frames == 0 || out_frames == 0 {
        return out;
    }
    let step = frames as f64 / out_frames as f64;
    for c in 0..channels {
        let src = &input[c * frames..(c + 1) * frames];
        let at = |i: isize| -> f32 { src[i.clamp(0, frames as isize - 1) as usize] };
        for i in 0..out_frames {
            let pos = i as f64 * step;
            let idx = pos.floor() as isize;
            let t = (pos - idx as f64) as f32;
            let (y0, y1, y2, y3) = (at(idx - 1), at(idx), at(idx + 1), at(idx + 2));
            let c1 = 0.5 * (y2 - y0);
            let c2 = y0 - 2.5 * y1 + 2.0 * y2 - 0.5 * y3;
            let c3 = 0.5 * (y3 - y0) + 1.5 * (y1 - y2);
            out[c * out_frames + i] = ((c3 * t + c2) * t + c1) * t + y1;
        }
    }
    out
}

/// Change speed by `rate` (2.0 = twice as fast) and pitch by `semitones`
/// independently. Output length is `round(frames / rate)`.
pub fn stretch_pitch(
    input: &[f32],
    frames: usize,
    channels: usize,
    sample_rate: f32,
    rate: f64,
    semitones: f64,
) -> (Vec<f32>, usize) {
    let rate = rate.clamp(0.25, 4.0);
    let ratio = 2f64.powf(semitones.clamp(-24.0, 24.0) / 12.0);
    let out_frames = ((frames as f64) / rate).round() as usize;
    let len_factor = ratio / rate;
    let mid_frames = ((frames as f64) * len_factor).round() as usize;

    let stretched = if (len_factor - 1.0).abs() < 1e-6 {
        input.to_vec()
    } else {
        wsola(input, frames, channels, sample_rate, len_factor, mid_frames)
    };
    if (ratio - 1.0).abs() < 1e-6 {
        return (stretched, mid_frames);
    }
    (resample(&stretched, mid_frames, channels, out_frames), out_frames)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn tone(freq: f32, sr: f32, len: usize) -> Vec<f32> {
        (0..len)
            .map(|i| (2.0 * std::f32::consts::PI * freq * i as f32 / sr).sin() * 0.5)
            .collect()
    }

    fn zero_crossings(x: &[f32]) -> usize {
        x.windows(2).filter(|w| w[0] <= 0.0 && w[1] > 0.0).count()
    }

    #[test]
    fn stretch_changes_length_not_pitch() {
        let sr = 44100.0;
        let x = tone(440.0, sr, 44100);
        let (y, n) = stretch_pitch(&x, x.len(), 1, sr, 0.5, 0.0);
        assert_eq!(n, 88200);
        assert_eq!(y.len(), 88200);
        let f = zero_crossings(&y[4410..n - 4410]) as f32 / ((n - 8820) as f32 / sr);
        assert!((f - 440.0).abs() < 15.0, "freq {f}");
    }

    #[test]
    fn pitch_shift_keeps_length() {
        let sr = 44100.0;
        let x = tone(440.0, sr, 44100);
        let (y, n) = stretch_pitch(&x, x.len(), 1, sr, 1.0, 12.0);
        assert_eq!(n, 44100);
        let f = zero_crossings(&y[4410..n - 4410]) as f32 / ((n - 8820) as f32 / sr);
        assert!((f - 880.0).abs() < 30.0, "freq {f}");
    }

    #[test]
    fn stereo_planar() {
        let sr = 22050.0;
        let mut x = tone(300.0, sr, 22050);
        x.extend(tone(300.0, sr, 22050));
        let (y, n) = stretch_pitch(&x, 22050, 2, sr, 2.0, -5.0);
        assert_eq!(n, 11025);
        assert_eq!(y.len(), 2 * n);
    }
}
