const stage  = document.querySelector('.stage');
const numEl  = document.getElementById('num');
const capEl  = document.getElementById('cap');
const bar    = document.querySelector('[role="progressbar"]');
const slider = document.getElementById('slider');
const runBtn = document.getElementById('run');
const reduce = matchMedia('(prefers-reduced-motion: reduce)').matches;

// Setting one value re-renders the whole fill.
function set(p) {
    p = Math.max(0, Math.min(100, p));
    stage.style.setProperty('--p', (p / 100).toFixed(4));
    numEl.textContent = Math.round(p);
    slider.value = p;
    bar.setAttribute('aria-valuenow', Math.round(p));
    capEl.textContent = p >= 99.5 ? 'Complete' : 'Loading';
    stage.classList.toggle('low', p < 20);
}

let raf;
function animateTo(target, dur) {
    cancelAnimationFrame(raf);
    if (reduce) { set(target); return; }
    const start = parseFloat(slider.value);
    const t0 = performance.now();
    const ease = t => 1 - Math.pow(1 - t, 3);   // ease-out cubic
    (function frame(now) {
        const t = Math.min(1, (now - t0) / dur);
        set(start + (target - start) * ease(t));
        if (t < 1) raf = requestAnimationFrame(frame);
    })(t0);
}

// slider.addEventListener('input', e => { cancelAnimationFrame(raf); set(+e.target.value); });
// runBtn.addEventListener('click', () => {
//     if (+slider.value >= 99.5) set(0);
//     animateTo(100, 3200);
// });

// gentle intro fill so the cube shows what it does on load
set(0);
requestAnimationFrame(() => animateTo(66, 2200));