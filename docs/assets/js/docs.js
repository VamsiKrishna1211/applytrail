// Progressive enhancement for the docs site. Nothing here is required to read
// a page: without it you get the same content, the same navigation, and no
// table of contents.
(function () {
  'use strict';

  var prose = document.querySelector('.prose');

  // --- wrap wide tables so the page body never scrolls sideways -----------
  if (prose) {
    prose.querySelectorAll('table').forEach(function (table) {
      if (table.parentElement.classList.contains('table-wrap')) return;
      var wrap = document.createElement('div');
      wrap.className = 'table-wrap';
      table.parentNode.insertBefore(wrap, table);
      wrap.appendChild(table);
    });
  }

  // --- table of contents, built from the headings kramdown already id'd ---
  var tocList = document.getElementById('toclist');
  var toc = document.getElementById('toc');
  var headings = prose ? Array.prototype.slice.call(
    prose.querySelectorAll('h2[id], h3[id]')) : [];

  if (tocList && headings.length >= 2) {
    headings.forEach(function (h) {
      var a = document.createElement('a');
      a.href = '#' + h.id;
      a.textContent = h.textContent.replace(/¶\s*$/, '').trim();
      a.className = h.tagName.toLowerCase();
      tocList.appendChild(a);

      // A quiet anchor, so a heading can be linked without a URL bar detour.
      var link = document.createElement('a');
      link.className = 'anchor';
      link.href = '#' + h.id;
      link.setAttribute('aria-label', 'Link to this section');
      link.textContent = '#';
      h.appendChild(link);
    });

    // Highlight the heading nearest the top of the viewport.
    var links = {};
    tocList.querySelectorAll('a').forEach(function (a) {
      links[a.getAttribute('href').slice(1)] = a;
    });
    var active = null;
    var spy = new IntersectionObserver(function () {
      var best = null;
      headings.forEach(function (h) {
        var top = h.getBoundingClientRect().top;
        if (top < 120 && (best === null || top > best.getBoundingClientRect().top)) best = h;
      });
      if (!best) best = headings[0];
      if (active === best) return;
      if (active && links[active.id]) links[active.id].classList.remove('active');
      active = best;
      if (links[active.id]) links[active.id].classList.add('active');
    }, { rootMargin: '-100px 0px -70% 0px', threshold: [0, 1] });
    headings.forEach(function (h) { spy.observe(h); });
  } else if (toc) {
    toc.classList.add('hidden');
  }

  // --- sidebar filter ----------------------------------------------------
  var filter = document.getElementById('navfilter');
  if (filter) {
    filter.addEventListener('input', function () {
      var q = filter.value.trim().toLowerCase();
      document.querySelectorAll('.sitenav ul').forEach(function (ul) {
        var shown = 0;
        ul.querySelectorAll('li').forEach(function (li) {
          var hit = !q || li.textContent.toLowerCase().indexOf(q) !== -1;
          li.classList.toggle('hidden', !hit);
          if (hit) shown++;
        });
        var head = ul.previousElementSibling;
        if (head && head.classList.contains('navgroup')) {
          head.classList.toggle('hidden', shown === 0);
        }
      });
    });
  }

  // --- mobile navigation -------------------------------------------------
  var toggle = document.querySelector('.navtoggle');
  var sidebar = document.getElementById('sidebar');
  if (toggle && sidebar) {
    toggle.addEventListener('click', function () {
      var open = sidebar.classList.toggle('open');
      toggle.setAttribute('aria-expanded', open ? 'true' : 'false');
    });
  }
})();
