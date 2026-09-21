// The page is a view over what Go hands across.  It keeps no state of its own
// beyond the checkboxes (which mods are ticked and which hooks are off)
// because everything else has one owner on the other side.

let hooks = [];

// The mods half comes from a poll rather than from the opening State: a mod
// installed while this window is open changes the folder under it.
let installed = [];
let offers = [];
let sources = [];

// The one thing the page decides. Seeded from the first poll and owned here
// afterwards, so a tick does not flicker back on the next one.
const enabled = new Set();
let seeded = false;

// What the page would hand over if Play were pressed right now.
function choice() {
  return [
    installed.filter(m => m.supported && enabled.has(m.id)).map(m => m.id),
    hooks.flatMap(g => g.hooks.filter(h => !h.enabled).map(h => h.name)),
    document.getElementById('flags').value.trim(),
    document.getElementById('debug').checked,
  ];
}

// Called on every change rather than only on Play.  Somebody who ticks a mod
// and then closes the window has still chosen something, and a launcher that
// forgets it feels broken in a way that is hard to describe and easy to notice.
let ready = false;

function remember() {
  if (ready) {
    window.smlSave(...choice());
  }
}

// The flag field is the one control that changes on every keystroke, and each
// change is a file being written.  A quarter of a second after the typing stops
// is still soon enough that closing the window keeps what was typed.
let typing = 0;

function rememberSoon() {
  clearTimeout(typing);
  typing = setTimeout(remember, 250);
}

function render() {
  const list = document.getElementById('mods');
  list.replaceChildren();
  document.getElementById('empty').hidden = installed.length > 0;
  document.getElementById('count').textContent = installed.length
    ? `${installed.filter(m => enabled.has(m.id) && m.supported).length} of ${installed.length} enabled`
    : '';

  for (const mod of installed) {
    const on = mod.supported && enabled.has(mod.id);
    const row = document.createElement('li');
    // A mod built against another version of the API is shown and cannot be
    // switched on. Hiding it would get us a bug report about a mod that
    // vanished.  This way the player sees it and reads why.
    row.className = mod.supported ? (on ? 'on' : '') : 'unsupported';

    const box = document.createElement('input');
    box.type = 'checkbox';
    box.checked = on;
    box.disabled = !mod.supported;

    const text = document.createElement('div');
    text.append(heading(mod), source(mod));
    if (mod.description) {
      text.append(line('mod-desc', mod.description));
    }
    // The reason comes from Go, because Go is what decided it: which of the
    // mod's two ranges this launcher falls outside of, and what it wrote.
    if (!mod.supported) {
      text.append(line('mod-warn', mod.refusal ||
        'This mod doesn’t support this version of the loader.'));
    }
    // Both are already in the folder, so there is nothing to hide and something
    // to say. The list of what to install hides such a pair instead.
    if (mod.conflict) {
      text.append(line('mod-warn', `This mod conflicts with ${mod.conflict}. ` +
        `Disable one of them to avoid unpredictable behavior.`));
    }

    const buttons = document.createElement('div');
    buttons.className = 'row-buttons';
    if (mod.update) {
      buttons.append(button(`Update to ${mod.update}`, '', () => {
        window.smlModsGet(mod.id);
      }));
    }
    buttons.append(button('Remove', 'ghost', () => window.smlModsDrop(mod.id)));

    // The whole row toggles: a 13px checkbox is a poor target and the row is
    // the thing that looks clickable.
    if (mod.supported) {
      row.addEventListener('click', () => {
        if (enabled.has(mod.id)) {
          enabled.delete(mod.id);
        } else {
          enabled.add(mod.id);
        }
        render();
        remember();
      });
    }

    row.append(picture(mod.id), box, text, buttons);
    list.append(row);
  }
}

// --- pieces every row is built out of ------------------------------------

function line(className, text) {
  const element = document.createElement('div');
  element.className = className;
  element.textContent = text;
  return element;
}

function heading(mod) {
  const name = line('mod-name', mod.name);
  if (mod.version) {
    const version = document.createElement('em');
    version.textContent = mod.version;
    name.append(version);
  }
  return name;
}

// Where a mod came from, small, under the name. Blank for one that is only in
// the folder: nothing is known about it and a made-up line would be worse.
function source(mod) {
  return line('mod-from', mod.source || '');
}

function button(text, className, onClick) {
  const element = document.createElement('button');
  element.type = 'button';
  element.className = className;
  element.textContent = text;
  element.addEventListener('click', event => {
    // The row underneath is a toggle, and pressing Remove is not a way of
    // saying "and also switch it on".
    event.stopPropagation();
    onClick();
  });
  return element;
}

// Icons arrive one at a time and long after the row does, so the row is drawn
// with the launcher's own mark and the picture is dropped in when it turns up.
// Nothing here loads a file: the page has no origin to be relative to, so what
// comes back from Go is a data URI.
const icons = new Map();

function picture(id) {
  const element = document.createElement('div');
  element.className = 'icon';
  element.dataset.id = id;
  const cached = icons.get(id);
  if (cached) {
    element.style.backgroundImage = `url("${cached}")`;
    element.classList.add('has');
  }
  return element;
}

async function sweepIcons(settled) {
  for (const element of document.querySelectorAll('.icon:not(.has)')) {
    const id = element.dataset.id;
    if (icons.has(id)) {
      continue;
    }
    const uri = await window.smlIcon(id);
    if (uri) {
      icons.set(id, uri);
      element.style.backgroundImage = `url("${uri}")`;
      element.classList.add('has');
    } else if (settled) {
      // Every repository has answered by now, so this mod has no picture and
      // asking again on every tick would be asking forever.
      icons.set(id, '');
    }
  }
}

// Hooks are drawn as chips rather than as a second list of rows: there are
// twenty of them against a handful of mods, and they are names to recognise
// rather than things to read.
function renderHooks() {
  const off = hooks.flatMap(g => g.hooks.filter(h => !h.enabled));
  document.getElementById('hookstate').textContent =
    off.length ? `${off.length} off` : 'all on';

  const box = document.getElementById('hooks');
  box.replaceChildren();
  for (const group of hooks) {
    const section = document.createElement('div');
    section.className = 'hgroup';

    const title = document.createElement('h3');
    title.textContent = group.module;
    // A module is the unit a crash usually gets narrowed to first, so its name
    // turns the whole group on or off: all on unless it already is.
    title.addEventListener('click', () => {
      const turnOn = group.hooks.some(h => !h.enabled);
      group.hooks.forEach(h => { h.enabled = turnOn; });
      renderHooks();
      remember();
    });
    section.append(title);

    const chips = document.createElement('div');
    chips.className = 'chips';
    for (const hook of group.hooks) {
      const chip = document.createElement('label');
      chip.className = hook.enabled ? 'chip on' : 'chip';

      const box = document.createElement('input');
      box.type = 'checkbox';
      box.checked = hook.enabled;

      const name = document.createElement('span');
      name.textContent = hook.name;

      chip.addEventListener('click', event => {
        // The label already forwards the click to its own checkbox.  Without
        // this the box would end up back where it started.
        event.preventDefault();
        hook.enabled = !hook.enabled;
        renderHooks();
        remember();
      });

      chip.append(box, name);
      chips.append(chip);
    }
    section.append(chips);
    box.append(section);
  }
}

// The game in this folder against the one every address in the loader was found
// in. A player who has the expected build sees nothing. Anyone else is told
// which is which while they can still do something about it, and Play is left
// alone, and the loader attaches to an unknown build rather than refusing it.
function showBuild(build) {
  if (!build || !build.exe || build.matches) {
    return;
  }
  const notice = document.getElementById('build');
  const label = document.createElement('strong');
  label.textContent = 'Different game build.';
  const found = build.version
    ? `${build.exe} ${build.version}`
    : `${build.exe}, with no version information`;
  notice.append(label, ` Found ${found}. This loader targets ` +
    `${build.expectedExe} ${build.expected}, so mods may not work correctly.`);
  notice.hidden = false;
}


// --- Java ----------------------------------------------------------------
// The one part of this page that changes while it is open, and the one part
// that talks to the network. Go does the talking. The page asks it to start
// something and then polls, because a binding that blocks blocks the window it
// is drawing the progress bar in.

let polling = 0;
let filled = false;

function size(count) {
  const mb = count / (1024 * 1024);
  return mb >= 1024 ? `${(mb / 1024).toFixed(2)} GB` : `${mb.toFixed(1)} MB`;
}

// The line above Play. Gold-dim when a JDK will be used and red when one will
// not, because those are two different pieces of news: which java is in use,
// and that nothing at all is going to load.
function showJava(java) {
  const line = document.getElementById('java-line');
  const button = document.getElementById('java-open');
  const label = document.createElement('strong');
  line.replaceChildren();

  if (java.usable) {
    line.classList.remove('missing');
    button.classList.remove('missing');
    label.textContent = `Java ${java.version}`;
    line.append(label, ` (${java.source}). Mods will use this version.`);
    button.textContent = 'Use another';
    return;
  }

  line.classList.add('missing');
  button.classList.add('missing');
  if (java.version) {
    label.textContent = `Java ${java.version} (${java.source}) is too old.`;
    line.append(label, ` The loader needs Java ${java.minimum} or newer. ` +
      `Sacred will start, but no mods will load.`);
  } else {
    label.textContent = 'Java was not found.';
    line.append(label, ` Sacred will start, but no mods will load. ` +
      `The loader needs Java ${java.minimum} or newer.`);
  }
  button.textContent = 'Get Java';
}

// Both dropdowns come from the index rather than from a list in here, so a
// distribution published next year is offered without a new launcher.
function fillCatalog(view) {
  if (!view.catalog.loaded || filled) {
    return;
  }
  filled = true;

  const vendors = document.getElementById('jdk-vendor');
  vendors.replaceChildren();
  for (const vendor of view.catalog.vendors) {
    const option = document.createElement('option');
    option.value = vendor.id;
    option.textContent = vendor.name;
    option.selected = vendor.id === view.vendor;
    vendors.append(option);
  }

  const versions = document.getElementById('jdk-version');
  versions.replaceChildren();
  for (const version of view.catalog.versions) {
    const option = document.createElement('option');
    option.value = String(version.major);
    option.textContent = version.lts ? `${version.major} (LTS)` : String(version.major);
    option.selected = version.major === view.version;
    versions.append(option);
  }
}

const stageWords = {
  catalog: () => 'Loading available JDKs from foojay',
  resolve: progress => progress.note,
  download: progress => progress.total
    ? `${progress.note}: ${size(progress.done)} of ${size(progress.total)}` +
      ` (${Math.floor((progress.done / progress.total) * 100)}%)`
    : `${progress.note}: ${size(progress.done)}`,
  extract: progress => progress.total
    ? `${progress.note}: ${Math.floor((progress.done / progress.total) * 100)}%`
    : progress.note,
  done: progress => progress.note,
};

function showProgress(view) {
  const progress = view.progress;
  const bar = document.getElementById('jdk-bar');
  const fill = document.getElementById('jdk-fill');
  const note = document.getElementById('jdk-note');
  const error = document.getElementById('jdk-error');

  // Nothing to download until the two dropdowns have something in them.
  document.getElementById('jdk-get').disabled = view.busy || !view.catalog.loaded;
  document.getElementById('jdk-vendor').disabled = view.busy;
  document.getElementById('jdk-version').disabled = view.busy;

  const trouble = progress.error || view.catalog.error;
  error.hidden = !trouble;
  error.textContent = trouble || '';

  const measured = progress.total > 0 &&
    (progress.stage === 'download' || progress.stage === 'extract');
  const running = view.busy || progress.stage === 'download' || progress.stage === 'extract';
  bar.hidden = !running;
  bar.classList.toggle('sweeping', running && !measured);
  fill.style.width = measured ? `${(progress.done / progress.total) * 100}%` : '';

  const say = stageWords[progress.stage];
  note.textContent = say ? say(progress) : '';
  note.classList.toggle('done', progress.stage === 'done');
}

async function tick() {
  const view = await window.smlJava();
  showJava(view.java);
  fillCatalog(view);
  showProgress(view);
  // Polling outlives the dialog: somebody who starts a 200 MB download and
  // closes the panel should still see the line above Play change when it lands.
  if (document.getElementById('jdk').hidden && !view.busy) {
    clearInterval(polling);
    polling = 0;
  }
}

function watch() {
  if (!polling) {
    polling = setInterval(tick, 200);
  }
  tick();
}

function wireJava(state) {
  showJava(state.java);

  document.getElementById('java-open').addEventListener('click', () => {
    document.getElementById('jdk').hidden = false;
    window.smlJavaLoad();
    watch();
  });

  document.getElementById('jdk-close').addEventListener('click', () => {
    document.getElementById('jdk').hidden = true;
  });

  document.getElementById('jdk-get').addEventListener('click', () => {
    const vendor = document.getElementById('jdk-vendor').value;
    const version = parseInt(document.getElementById('jdk-version').value, 10);
    if (!vendor || !version) {
      return;
    }
    window.smlJavaGet(vendor, version);
    watch();
  });
}


// --- the mod store -------------------------------------------------------
// Repositories are asked over the network, so this half works the way the Java
// panel does: Go is told to start something and the page polls for what to
// draw. The redraw only happens when the answer changed, which is what stops a
// list rebuilding itself under somebody's cursor.

let storeSeen = '';
let sealed = false;

function renderOffers() {
  const list = document.getElementById('offers');
  const nothing = document.getElementById('offers-empty');
  list.replaceChildren();

  nothing.hidden = offers.length > 0;
  nothing.textContent = sources.some(s => s.error)
    ? 'No mods available. A repository did not respond. Check Repositories.'
    : 'All available mods are already installed.';

  for (const mod of offers) {
    const row = document.createElement('li');
    row.className = mod.supported ? 'offer' : 'offer unsupported';

    const text = document.createElement('div');
    text.append(heading(mod), source(mod));
    if (mod.description) {
      text.append(line('mod-desc', mod.description));
    }
    if (!mod.supported) {
      text.append(line('mod-warn', mod.refusal ||
        'This mod was built for a different loader version and cannot run here.'));
    }

    const buttons = document.createElement('div');
    buttons.className = 'row-buttons';
    if (mod.supported) {
      buttons.append(button('Install', '', () => window.smlModsGet(mod.id)));
    }
    if (mod.size) {
      buttons.append(line('mod-size', weight(mod.size)));
    }

    row.append(picture(mod.id), text, buttons);
    list.append(row);
  }
}

function renderSources() {
  const list = document.getElementById('source-list');
  list.replaceChildren();
  for (const one of sources) {
    const row = document.createElement('li');

    const text = document.createElement('div');
    text.append(line('mod-name', one.name || one.label));
    text.append(line('mod-from', one.label));
    if (one.error) {
      text.append(line('mod-warn', one.error));
    } else {
      text.append(line('mod-desc',
        `${one.mods} mod${one.mods === 1 ? '' : 's'}` +
        (one.token ? ', using an access token' : '')));
    }
    row.append(text);

    // The official one has no Remove: a launcher with no repositories at all is
    // a page with nothing on it and no way back to one.
    if (one.removable) {
      row.append(button('Remove', 'ghost', () => window.smlSourceDrop(one.url)));
    }
    list.append(row);
  }
}

// A mod jar is kilobytes when it is plain Java and megabytes when it brings a
// toolkit along inside it. The JDK panel's formatter starts at megabytes, and
// "0.0 MB" is not a size.
function weight(count) {
  const kb = count / 1024;
  return kb >= 1024 ? `${(kb / 1024).toFixed(1)} MB` : `${Math.max(1, Math.round(kb))} KB`;
}

const storeWords = {
  index: progress => `Loading ${progress.note}`,
  download: progress => progress.total
    ? `${progress.note}: ${Math.floor((progress.done / progress.total) * 100)}%`
    : progress.note,
  done: progress => `${progress.note} installed`,
};

function showStore(view) {
  const progress = view.progress;
  const bar = document.getElementById('store-bar');
  const note = document.getElementById('store-note');
  const error = document.getElementById('store-error');

  error.hidden = !progress.error;
  error.textContent = progress.error || '';

  const measured = progress.stage === 'download' && progress.total > 0;
  bar.hidden = !view.busy;
  bar.classList.toggle('sweeping', view.busy && !measured);
  document.getElementById('store-fill').style.width =
    measured ? `${(progress.done / progress.total) * 100}%` : '';

  const say = storeWords[progress.stage];
  const speaking = view.busy || progress.stage === 'done';
  note.textContent = speaking && say ? say(progress) : '';
  note.classList.toggle('done', progress.stage === 'done');

  document.getElementById('refresh').disabled = view.busy;
  document.getElementById('source-add').disabled = view.busy;
}

function tab(which) {
  const showing = which === 'available';
  document.getElementById('page-available').hidden = !showing;
  document.getElementById('page-installed').hidden = showing;
  document.getElementById('tab-available').classList.toggle('on', showing);
  document.getElementById('tab-installed').classList.toggle('on', !showing);
}

async function storeTick() {
  const view = await window.smlMods();
  const seen = JSON.stringify(view);
  if (seen !== storeSeen) {
    storeSeen = seen;
    installed = view.installed || [];
    offers = view.offers || [];
    sources = view.sources || [];
    if (!seeded) {
      // The first answer is the settings file's opinion of what is on. After
      // that this page owns it.
      seeded = true;
      installed.filter(m => m.enabled).forEach(m => enabled.add(m.id));
      // Somebody with an empty mods folder wants to see what they could have,
      // not an empty box and a tab to go and find.
      if (installed.length === 0) {
        tab('available');
      }
    }
    render();
    renderOffers();
    renderSources();
  }
  showStore(view);
  await sweepIcons(view.loaded && !view.busy);
}

function wireStore() {
  document.getElementById('tab-installed').addEventListener('click', () => tab('installed'));
  document.getElementById('tab-available').addEventListener('click', () => tab('available'));
  document.getElementById('refresh').addEventListener('click', () => {
    icons.clear();
    window.smlModsRefresh();
  });

  const dialog = document.getElementById('sources');
  document.getElementById('sources-open').addEventListener('click', () => {
    dialog.hidden = false;
  });
  document.getElementById('sources-close').addEventListener('click', () => {
    dialog.hidden = true;
  });
  document.getElementById('source-add').addEventListener('click', () => {
    const url = document.getElementById('source-url');
    const token = document.getElementById('source-token');
    if (!url.value.trim()) {
      return;
    }
    window.smlSourceAdd(url.value.trim(), token.value);
    url.value = '';
    token.value = '';
  });

  // What happens to a token is a promise, so the page says which promise it is
  // actually able to make on this machine.
  document.getElementById('source-hint').textContent = sealed
    ? 'Access tokens are encrypted with your Windows account before they are ' +
      'saved. They cannot be used from another account or computer. Public ' +
      'repositories do not need a token.'
    : 'Windows cannot encrypt access tokens on this computer. Any token you ' +
      'enter will be saved as plain text in launcher.json. Leave this field ' +
      'empty unless you accept that risk.';

  setInterval(storeTick, 500);
}

// --- updating the launcher -----------------------------------------------
// The launcher is one file with the whole loader inside it, so upgrading it is
// downloading one file and starting it. Go does both; this half draws a line
// in the header and a dialog with a bar in it.

let watchingUpdate = 0;
// Whether a downloaded launcher is already waiting. Kept here so the link can
// tell "fetch it" from "it is fetched" without another round trip.
let updateStaged = false;
// The check runs once at startup and takes as long as one request to
// api.github.com. This gives up after a minute rather than polling a launcher
// somebody left open all evening.
let checksLeft = 60;

const updateWords = {
  download: p => p.total
    ? `Downloading ${size(p.done)} of ${size(p.total)}`
    : `Downloading ${size(p.done)}`,
  install: p => p.note,
  ready: p => p.note,
};

function showUpdate(view) {
  updateStaged = view.staged;
  const line = document.getElementById('update');
  line.hidden = !view.found;
  if (!view.found) {
    return;
  }
  document.getElementById('update-what').textContent =
    `Version ${view.release.version} is available.`;
  document.getElementById('update-open').textContent =
    view.staged ? 'Install now' : 'Download & install now';
}

function showUpdateProgress(view) {
  const progress = view.progress;
  const bar = document.getElementById('update-bar');
  const fill = document.getElementById('update-fill');
  const note = document.getElementById('update-note');
  const error = document.getElementById('update-error');
  const rescue = document.getElementById('update-rescue');
  const page = document.getElementById('update-page');

  // No way out while bytes are moving, and no way to press the button twice.
  document.getElementById('update-close').hidden = view.busy;
  const go = document.getElementById('update-go');
  go.disabled = view.busy || !view.staged;

  const failed = progress.stage === 'failed';
  error.hidden = !failed;
  error.textContent = failed
    ? `${progress.error} Try again later, or download it yourself:`
    : '';
  rescue.hidden = !failed;
  page.textContent = view.release.page || '';

  const running = view.busy || view.staged;
  const measured = progress.total > 0;
  bar.hidden = !running;
  bar.classList.toggle('sweeping', running && !measured);
  fill.style.width = measured ? `${(progress.done / progress.total) * 100}%` : '';

  const say = updateWords[progress.stage];
  note.textContent = say ? say(progress) : '';
  note.classList.toggle('done', progress.stage === 'ready');
}

async function updateTick() {
  const view = await window.smlUpdate();
  showUpdate(view);
  showUpdateProgress(view);
  // The header line is the whole point of the early poll, so it stops once
  // there is an answer. It keeps going while the dialog is doing something.
  const working = view.busy || !document.getElementById('updater').hidden;
  if (!working && (view.found || --checksLeft <= 0)) {
    clearInterval(watchingUpdate);
    watchingUpdate = 0;
  }
}

function watchUpdate() {
  if (!watchingUpdate) {
    watchingUpdate = setInterval(updateTick, 1000);
  }
  updateTick();
}

function wireUpdate() {
  document.getElementById('update-open').addEventListener('click', () => {
    document.getElementById('updater').hidden = false;
    // Straight into the download. The player pressed a link that says what it
    // is going to do, and a dialog that then asks again is a dialog nobody
    // reads the second time. Unless it is already downloaded, in which case
    // the only thing left is the button underneath.
    if (!updateStaged) {
      window.smlUpdateGet();
    }
    watchUpdate();
  });

  document.getElementById('update-close').addEventListener('click', () => {
    document.getElementById('updater').hidden = true;
  });

  document.getElementById('update-go').addEventListener('click', () => {
    window.smlUpdateApply();
    watchUpdate();
  });

  document.getElementById('update-page').addEventListener('click', () => {
    window.smlOpen(document.getElementById('update-page').textContent);
  });

  watchUpdate();
}


async function start() {
  const state = await window.smlState();
  hooks = state.hooks || [];
  sealed = !!state.sealed;
  document.getElementById('version').textContent = `Version ${state.version}`;
  showBuild(state.game);
  document.getElementById('flags').value = state.flags || '';
  document.getElementById('debug').checked = !!state.debug;
  wireJava(state);
  wireStore();
  wireUpdate();
  // Before `ready` below, so the first thing this page remembers is the
  // settings file's own answer rather than an empty list.
  await storeTick();
  renderHooks();

  const advanced = document.getElementById('advanced');
  advanced.hidden = hooks.length === 0;
  const disclose = document.getElementById('disclose');
  disclose.addEventListener('click', () => {
    const open = !advanced.classList.contains('open');
    advanced.classList.toggle('open', open);
    disclose.setAttribute('aria-expanded', String(open));
  });

  // input rather than change: a player who types a flag and closes the window
  // without leaving the field would otherwise lose it.
  document.getElementById('flags').addEventListener('input', rememberSoon);
  document.getElementById('debug').addEventListener('change', remember);
  ready = true;

  document.getElementById('play').addEventListener('click', () => {
    const button = document.getElementById('play');
    button.disabled = true;
    button.textContent = 'Playing';
    window.smlPlay(...choice()).then(() => {
      button.disabled = false;
      button.textContent = 'Play';
    });
  });
}

start();
