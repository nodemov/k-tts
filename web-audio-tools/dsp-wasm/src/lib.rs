//! C-ABI exports consumed from JavaScript.
//!
//! Buffers are planar `f32` (`channels * frames`). JS allocates with
//! `alloc_f32`, copies samples into linear memory, calls a processing
//! function and reads the result back before calling `free_f32`.

pub mod denoise;
pub mod fft;
pub mod stretch;

use std::slice;

#[no_mangle]
pub extern "C" fn alloc_f32(len: usize) -> *mut f32 {
    let mut v = Vec::<f32>::with_capacity(len.max(1));
    let ptr = v.as_mut_ptr();
    std::mem::forget(v);
    ptr
}

/// # Safety
/// `ptr` must come from `alloc_f32` with the same `len`.
#[no_mangle]
pub unsafe extern "C" fn free_f32(ptr: *mut f32, len: usize) {
    drop(Vec::from_raw_parts(ptr, 0, len.max(1)));
}

/// In-place noise reduction. `strength` is 0..1.
///
/// # Safety
/// `ptr` must point to `channels * frames` initialised floats.
#[no_mangle]
pub unsafe extern "C" fn denoise(
    ptr: *mut f32,
    frames: usize,
    channels: usize,
    sample_rate: f32,
    strength: f32,
) {
    let data = slice::from_raw_parts_mut(ptr, frames * channels);
    for c in 0..channels {
        denoise::denoise_channel(&mut data[c * frames..(c + 1) * frames], sample_rate, strength);
    }
}

/// Number of output frames `stretch_pitch` will write.
#[no_mangle]
pub extern "C" fn stretch_out_frames(frames: usize, rate: f64) -> usize {
    ((frames as f64) / rate.clamp(0.25, 4.0)).round() as usize
}

/// Time-stretch by `rate` and pitch-shift by `semitones`.
///
/// # Safety
/// `input` holds `channels * frames` floats and `output` has room for
/// `channels * stretch_out_frames(frames, rate)` floats.
#[no_mangle]
pub unsafe extern "C" fn stretch_pitch(
    input: *const f32,
    frames: usize,
    channels: usize,
    sample_rate: f32,
    rate: f64,
    semitones: f64,
    output: *mut f32,
) -> usize {
    let src = slice::from_raw_parts(input, frames * channels);
    let (result, out_frames) =
        stretch::stretch_pitch(src, frames, channels, sample_rate, rate, semitones);
    let dst = slice::from_raw_parts_mut(output, out_frames * channels);
    dst.copy_from_slice(&result[..out_frames * channels]);
    out_frames
}
