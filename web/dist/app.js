// GALA — reproductor web mínimo (Brief M4 §6). HTML+JS plano, sin
// framework: SvelteKit llega en M10, esto solo demuestra que M4 funciona.
(function () {
  'use strict';

  var gallery = document.getElementById('gallery');
  var playerView = document.getElementById('playerView');
  var playerTitle = document.getElementById('playerTitle');
  var video = document.getElementById('video');
  var sourceList = document.getElementById('sourceList');
  var rejectPanel = document.getElementById('rejectPanel');
  var rejectReason = document.getElementById('rejectReason');
  var copyLinkBtn = document.getElementById('copyLinkBtn');
  var copyPathBtn = document.getElementById('copyPathBtn');
  var analyzingPanel = document.getElementById('analyzingPanel');
  var backBtn = document.getElementById('backBtn');
  var toastEl = document.getElementById('toast');
  var ffmpegBadge = document.getElementById('ffmpegBadge');

  var hls = null;
  var currentItemId = null;
  var currentSources = []; // fuentes de /play, en el orden ya decidido por el servidor
  var currentSourceIdx = -1;
  var saveTimer = null;
  // remuxTimeOffset: en modo remux cada sesión HLS arranca en su propio
  // tiempo 0 (ffmpeg -ss antes de -i), así que video.currentTime es relativo
  // a la SESIÓN, no al archivo. Se suma aquí para guardar el progreso
  // absoluto real (Brief M4 §4/§6).
  var remuxTimeOffset = 0;
  // knownDurationS: duración TOTAL real del archivo (de media_info, vía
  // /play), para no confundirla con video.duration de una sesión de remux
  // que arrancó a mitad de archivo (esa sería más corta que el total).
  var knownDurationS = 0;
  var currentRejectSource = null; // fuente mostrada en rejectPanel, para los botones "copiar"
  var analyzingRetried = false; // un solo reintento automático por fuente en análisis

  function toast(msg) {
    toastEl.textContent = msg;
    toastEl.hidden = false;
    clearTimeout(toastEl._t);
    toastEl._t = setTimeout(function () { toastEl.hidden = true; }, 4000);
  }

  function fmtYear(it) { return it.year ? ' (' + it.year + ')' : ''; }

  // --- Galería -----------------------------------------------------------

  function loadGallery() {
    fetch('/api/v1/items?limit=200')
      .then(function (r) { return r.json(); })
      .then(function (res) { renderGallery(res.items || []); })
      .catch(function () { gallery.innerHTML = '<p class="hint">No se pudo cargar la galería.</p>'; });
  }

  function renderGallery(items) {
    gallery.innerHTML = '';
    if (items.length === 0) {
      gallery.innerHTML = '<p class="hint">Sin contenido catalogado todavía.</p>';
      return;
    }
    items.forEach(function (it) {
      var card = document.createElement('div');
      card.className = 'card';
      card.tabIndex = 0;

      var thumb = document.createElement('div');
      thumb.className = 'thumb';
      if (it.poster_path) {
        var img = document.createElement('img');
        img.loading = 'lazy';
        img.src = '/api/v1/img/' + encodeURIComponent(it.poster_path);
        img.alt = it.title;
        thumb.appendChild(img);
      } else {
        thumb.textContent = '🎬';
      }
      if (it.progress && it.progress.duration_s) {
        var pct = Math.min(100, (it.progress.position_s / it.progress.duration_s) * 100);
        var bar = document.createElement('div');
        bar.className = 'progress';
        var fill = document.createElement('div');
        fill.style.width = pct + '%';
        bar.appendChild(fill);
        thumb.appendChild(bar);
      }
      card.appendChild(thumb);

      var title = document.createElement('div');
      title.className = 'title';
      title.textContent = it.title + fmtYear(it);
      card.appendChild(title);

      var open = function () { openPlayer(it); };
      card.addEventListener('click', open);
      card.addEventListener('keyup', function (e) { if (e.key === 'Enter') open(); });
      gallery.appendChild(card);
    });
  }

  // --- Reproductor ---------------------------------------------------------

  function openPlayer(item) {
    currentItemId = item.id;
    analyzingRetried = false;
    playerTitle.textContent = item.title + fmtYear(item);
    gallery.hidden = true;
    playerView.hidden = false;
    rejectPanel.hidden = true;
    analyzingPanel.hidden = true;
    video.hidden = false;
    sourceList.innerHTML = '<p class="hint">Cargando fuentes…</p>';
    fetchPlay(item);
  }

  // fetchPlay es el GET a /play propiamente dicho, sin reiniciar el estado
  // de "ya reintentado una vez" — lo usa tanto openPlayer (primera vez) como
  // el reintento automático de ModeAnalyzing (addendum §1).
  function fetchPlay(item) {
    fetch('/api/v1/play/' + item.id)
      .then(function (r) { return r.json(); })
      .then(function (data) { onPlayData(data, item); })
      .catch(function () { sourceList.innerHTML = '<p class="hint">No se pudo consultar /play.</p>'; });
  }

  function onPlayData(data, item) {
    currentSources = data.sources || [];
    ffmpegBadge.hidden = !!data.ffmpeg_available;

    if (currentSources.length === 0) {
      video.hidden = true;
      analyzingPanel.hidden = true;
      rejectPanel.hidden = false;
      rejectReason.textContent = 'No hay ninguna fuente disponible para este título.';
      currentRejectSource = null;
      copyLinkBtn.hidden = true;
      copyPathBtn.hidden = true;
      sourceList.innerHTML = '';
      return;
    }

    renderSourceList();
    var startIdx = 0;
    if (data.chosen_source_id) {
      var idx = currentSources.findIndex(function (s) { return s.source_id === data.chosen_source_id; });
      if (idx >= 0) startIdx = idx;
    }
    selectSource(startIdx, item);
  }

  function renderSourceList() {
    sourceList.innerHTML = '';
    if (currentSources.length < 2) return; // una sola fuente: no hace falta selector
    currentSources.forEach(function (s, idx) {
      var btn = document.createElement('button');
      btn.className = 'source-btn';
      var label = s.provider + (s.quality_label ? ' · ' + s.quality_label : '') + ' · ' + s.health;
      if (s.mode === 'reject') label += ' · no reproducible';
      else if (s.mode === 'analyzing') label += ' · analizando';
      btn.textContent = label;
      btn.addEventListener('click', function () { selectSource(idx, { id: currentItemId }); });
      sourceList.appendChild(btn);
    });
  }

  function markActiveSourceButton(idx) {
    var btns = sourceList.querySelectorAll('.source-btn');
    btns.forEach(function (b, i) { b.classList.toggle('active', i === idx); });
  }

  // buildRejectMessage construye un mensaje honesto y neutro a partir del
  // "reason" que ya devuelve la API — nunca se hardcodea un códec concreto
  // aquí (Brief M4 addendum §3).
  function buildRejectMessage(source) {
    var base = source.reason || 'Este archivo no se puede reproducir en el navegador';
    return base + '. Próximamente GALA podrá convertirlo automáticamente.';
  }

  function showRejectPanel(source, message) {
    video.hidden = true;
    analyzingPanel.hidden = true;
    rejectPanel.hidden = false;
    rejectReason.textContent = message;
    currentRejectSource = source;
    copyLinkBtn.hidden = !source;
    copyPathBtn.hidden = !(source && source.path);
  }

  function selectSource(idx, item) {
    if (idx < 0 || idx >= currentSources.length) {
      showRejectPanel(null, 'Ninguna fuente pudo reproducirse.');
      return;
    }
    currentSourceIdx = idx;
    markActiveSourceButton(idx);
    var source = currentSources[idx];
    knownDurationS = (source.media && source.media.duration_s) || 0;
    stopHls();

    if (source.mode === 'reject') {
      showRejectPanel(source, buildRejectMessage(source));
      return;
    }

    if (source.mode === 'analyzing') {
      video.hidden = true;
      rejectPanel.hidden = true;
      analyzingPanel.hidden = false;
      // Un único reintento automático: el análisis suele resolverse dentro
      // de la misma petición a /play; si sigue pendiente, no se insiste más
      // (Brief M4 addendum §1: estado transitorio, no un rechazo).
      if (!analyzingRetried) {
        analyzingRetried = true;
        setTimeout(function () {
          if (currentItemId === item.id) fetchPlay(item);
        }, 1500);
      }
      return;
    }

    video.hidden = false;
    rejectPanel.hidden = true;
    analyzingPanel.hidden = true;
    analyzingRetried = false;

    if (source.mode === 'direct') {
      remuxTimeOffset = 0;
      video.src = source.url;
      resumeFromSavedProgress(item, function (pos) {
        if (pos > 0) {
          video.addEventListener('loadedmetadata', function once() {
            video.removeEventListener('loadedmetadata', once);
            video.currentTime = pos;
          });
        }
        video.play().catch(function () {});
      });
    } else if (source.mode === 'remux') {
      // Primera vez que se elige esta fuente (no un re-seek): arrancar ya
      // directamente en el punto guardado, en vez de arrancar en 0 y saltar
      // (evita una sesión de remux desechada de inmediato).
      resumeFromSavedProgress(item, function (pos) { startRemux(source, pos, item); });
    }
  }

  // resumeFromSavedProgress consulta /playback/{item} y llama a cb(segundos),
  // 0 si no hay progreso guardado o ya está visto (Brief M4 §6: "reanudar
  // desde progress al abrir").
  function resumeFromSavedProgress(item, cb) {
    fetch('/api/v1/playback/' + item.id).then(function (r) { return r.json(); }).then(function (p) {
      cb(p && !p.watched && p.position_s > 5 ? p.position_s : 0);
    }).catch(function () { cb(0); });
  }

  function startRemux(source, startSeconds, item) {
    var url = source.url + (startSeconds > 0 ? ('?t=' + startSeconds.toFixed(1)) : '');
    fetch(url).then(function (r) {
      if (!r.ok) { throw new Error('hls/start ' + r.status); }
      return r.json();
    }).then(function (data) {
      remuxTimeOffset = startSeconds;
      attachHls(data.playlist_url);
      video.play().catch(function () {});
    }).catch(function (err) {
      toast('No se pudo preparar la conversión: ' + err.message);
      onSourceFailed();
    });
  }

  function attachHls(playlistUrl) {
    if (window.Hls && window.Hls.isSupported()) {
      hls = new window.Hls();
      hls.loadSource(playlistUrl);
      hls.attachMedia(video);
    } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
      video.src = playlistUrl; // Safari: HLS nativo
    } else {
      toast('Este navegador no soporta HLS.');
    }
  }

  function stopHls() {
    if (hls) { hls.destroy(); hls = null; }
    video.removeAttribute('src');
    video.load();
  }


  // --- Fallo de fuente → siguiente automáticamente (Brief M4 §6) --------

  function onSourceFailed() {
    var failedSource = currentSources[currentSourceIdx];
    if (failedSource) {
      toast('Fallo al reproducir "' + failedSource.provider + '": probando la siguiente fuente…');
      fetch('/api/v1/sources/' + failedSource.source_id + '/failed', { method: 'POST' }).catch(function () {});
    }
    var next = nextPlayableIndex(currentSourceIdx);
    selectSource(next, { id: currentItemId });
  }

  function nextPlayableIndex(fromIdx) {
    for (var i = fromIdx + 1; i < currentSources.length; i++) {
      if (currentSources[i].mode !== 'reject' && currentSources[i].mode !== 'analyzing') return i;
    }
    return -1;
  }

  video.addEventListener('error', function () {
    if (currentSourceIdx >= 0) onSourceFailed();
  });

  // Seeking más allá de lo ya remuxeado: arrancar una sesión nueva desde ahí
  // (Brief M4 §4/§6: "el cliente pide desde otro punto; arranque con -ss
  // creando sesión nueva").
  video.addEventListener('seeking', function () {
    if (currentSourceIdx < 0) return;
    var source = currentSources[currentSourceIdx];
    if (!source || source.mode !== 'remux') return;
    var t = video.currentTime;
    var covered = false;
    var buf = video.buffered;
    for (var i = 0; i < buf.length; i++) {
      if (t >= buf.start(i) - 1 && t <= buf.end(i) + 1) { covered = true; break; }
    }
    if (!covered) {
      startRemux(source, t, { id: currentItemId });
    }
  });

  // --- Progreso: cada 10s y al pausar (Brief M4 §6) -----------------------

  function saveProgress() {
    var duration = knownDurationS || video.duration;
    if (!currentItemId || !duration || isNaN(duration)) return;
    fetch('/api/v1/playback/' + currentItemId, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ position_s: video.currentTime + remuxTimeOffset, duration_s: duration }),
    }).catch(function () {});
  }

  video.addEventListener('play', function () {
    clearInterval(saveTimer);
    saveTimer = setInterval(saveProgress, 10000);
  });
  video.addEventListener('pause', function () {
    clearInterval(saveTimer);
    saveProgress();
  });

  // --- Navegación ----------------------------------------------------------

  backBtn.addEventListener('click', function () {
    stopHls();
    clearInterval(saveTimer);
    video.pause();
    currentItemId = null;
    currentSourceIdx = -1;
    playerView.hidden = true;
    gallery.hidden = false;
    loadGallery(); // refresca progreso/miniaturas
  });

  function absoluteURL(path) {
    return window.location.origin + path;
  }

  // --- Copiar enlace / ruta (Brief M4 addendum §3: sin nombrar marcas) ---

  // copyToClipboard usa la Clipboard API cuando está disponible (requiere
  // contexto seguro: https o localhost) y cae a execCommand('copy') si no
  // —relevante en Modo LAN (M7), donde el origen es http:// simple y la
  // Clipboard API moderna puede no estar disponible.
  function copyToClipboard(text) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(text);
    }
    return new Promise(function (resolve, reject) {
      var ta = document.createElement('textarea');
      ta.value = text;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.focus();
      ta.select();
      try {
        var ok = document.execCommand('copy');
        document.body.removeChild(ta);
        if (ok) { resolve(); } else { reject(new Error('execCommand copy falló')); }
      } catch (e) {
        document.body.removeChild(ta);
        reject(e);
      }
    });
  }

  copyLinkBtn.addEventListener('click', function () {
    if (!currentRejectSource) return;
    copyToClipboard(absoluteURL(currentRejectSource.url))
      .then(function () { toast('Enlace copiado.'); })
      .catch(function () { toast('No se pudo copiar el enlace.'); });
  });
  copyPathBtn.addEventListener('click', function () {
    if (!currentRejectSource || !currentRejectSource.path) return;
    copyToClipboard(currentRejectSource.path)
      .then(function () { toast('Ruta copiada.'); })
      .catch(function () { toast('No se pudo copiar la ruta.'); });
  });

  // --- Arranque --------------------------------------------------------------

  fetch('/api/v1/ping').catch(function () {});
  loadGallery();
})();
