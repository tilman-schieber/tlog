// Colours the pieces of tlog's screen shown in the docs the way the outliner
// colours them: faint bullets, blue links, magenta tags, a yellow open box.
// Only ```text blocks are touched, and only their colour: the characters stay
// exactly as written. The classes are styled in tlog.css.
(function () {
    const escape = s => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    const span = (cls, s) => '<span class="' + cls + '">' + escape(s) + '</span>';

    const inline = new RegExp([
        '(\\[\\[[^\\]\\n]*\\]\\])',            // 1 [[a page]] or [[a day]]
        '(\\[[^\\]\\n]+\\]\\([^)\\n]+\\))',    // 2 [a label](an address)
        '(^|\\s)(#[\\p{L}\\d][\\p{L}\\d_-]*)', // 3, 4 #tag
        '(☐)',                                 // 5
        '(☑)',                                 // 6
        '(⏰ \\S+(?: \\S+)?)',                 // 7 ⏰ Fr 20.11.
        '(\\^[a-z0-9]{6})\\b',                 // 8 ^anchor
    ].join('|'), 'gu');

    function colour(line) {
        let note = '';
        const at = line.search(/\s+← /);
        if (at >= 0) {
            note = span('t-note', line.slice(at));
            line = line.slice(0, at);
        }
        let head = '';
        const bullet = /^(\s*)([•▾▸])(?= |$)/.exec(line);
        if (bullet) {
            head = bullet[1] + span('t-bullet', bullet[2]);
            line = line.slice(bullet[0].length);
        }
        let out = '';
        let last = 0;
        for (const m of line.matchAll(inline)) {
            out += escape(line.slice(last, m.index));
            if (m[1]) out += span('t-link', m[1]);
            else if (m[2]) out += span('t-url', m[2]);
            else if (m[4]) out += escape(m[3]) + span('t-tag', m[4]);
            else if (m[5]) out += span('t-todo', m[5]);
            else if (m[6]) out += span('t-done', m[6]);
            else if (m[7]) out += span('t-date', m[7]);
            else if (m[8]) out += span('t-anchor', m[8]);
            last = m.index + m[0].length;
        }
        return head + out + escape(line.slice(last)) + note;
    }

    for (const block of document.querySelectorAll('pre > code.language-text')) {
        block.innerHTML = block.textContent.split('\n').map(colour).join('\n');
    }
})();
