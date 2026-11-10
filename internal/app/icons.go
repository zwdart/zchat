package app

import "github.com/egoist/mygo/ui"

// Icons drawn on a 24-point grid in the color of the text.
func strokeIcon(paths string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">` + paths + `</svg>`))
}

func fillIcon(paths string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor">` + paths + `</svg>`))
}

var (
	iconChat        = strokeIcon(`<path d="M21 11.5a8.4 8.4 0 0 1-9 8.4 9.2 9.2 0 0 1-3.5-.7L3 21l1.8-5.4A8.4 8.4 0 1 1 21 11.5z"/>`)
	iconChatFill    = fillIcon(`<path d="M12 2a9.9 9.9 0 0 0-8.4 15.2L2 22l4.9-1.5A9.9 9.9 0 1 0 12 2z"/>`)
	iconUsers       = strokeIcon(`<circle cx="9" cy="8" r="3.2"/><path d="M2.8 21a6.2 6.2 0 0 1 12.4 0"/><path d="M16.5 4.8a3.2 3.2 0 0 1 0 6.4"/><path d="M18 14.5a6.2 6.2 0 0 1 3.2 6.5"/>`)
	iconCompass     = strokeIcon(`<circle cx="12" cy="12" r="9"/><path d="m15.5 8.5-2 5-5 2 2-5z"/>`)
	iconFolder      = strokeIcon(`<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>`)
	iconShare       = strokeIcon(`<circle cx="18" cy="5" r="2.5"/><circle cx="6" cy="12" r="2.5"/><circle cx="18" cy="19" r="2.5"/><path d="m8.2 10.8 7.6-4.4"/><path d="m8.2 13.2 7.6 4.4"/>`)
	iconSend        = strokeIcon(`<path d="m22 2-7 20-4-9-9-4z"/><path d="M22 2 11 13"/>`)
	iconImage       = strokeIcon(`<rect x="3" y="3" width="18" height="18" rx="3"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="m21 15-5-5L5 21"/>`)
	iconClip        = strokeIcon(`<path d="m21.4 11.6-8.9 8.9a5.5 5.5 0 0 1-7.8-7.8l8.9-8.9a3.7 3.7 0 0 1 5.2 5.2l-8.9 8.9a1.8 1.8 0 0 1-2.6-2.6l8.2-8.2"/>`)
	iconSearch      = strokeIcon(`<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>`)
	iconPlus        = strokeIcon(`<path d="M12 5v14"/><path d="M5 12h14"/>`)
	iconX           = strokeIcon(`<path d="M18 6 6 18"/><path d="m6 6 12 12"/>`)
	iconCheck       = strokeIcon(`<path d="m5 12 4 4L19 6"/>`)
	iconMore        = fillIcon(`<circle cx="5" cy="12" r="1.9"/><circle cx="12" cy="12" r="1.9"/><circle cx="19" cy="12" r="1.9"/>`)
	iconCopy        = strokeIcon(`<rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15H4a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1v1"/>`)
	iconDownload    = strokeIcon(`<path d="M12 3v12"/><path d="m7 10 5 5 5-5"/><path d="M4 21h16"/>`)
	iconUpload      = strokeIcon(`<path d="M12 21V9"/><path d="m7 14 5-5 5 5"/><path d="M4 3h16"/>`)
	iconTrash       = strokeIcon(`<path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>`)
	iconRefresh     = strokeIcon(`<path d="M20 7v5h-5"/><path d="M20 12a8 8 0 1 0-2.3 5.7"/>`)
	iconSettings    = strokeIcon(`<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 0 1-4 0v-.1a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.9 1.7 1.7 0 0 0-1.5-1H3a2 2 0 0 1 0-4h.1a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.9.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 0 1 4 0v.1a1.7 1.7 0 0 0 1 1.6 1.7 1.7 0 0 0 1.9-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.9V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 0 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/>`)
	iconEmoji       = strokeIcon(`<circle cx="12" cy="12" r="9"/><path d="M8.5 14.5a4.5 4.5 0 0 0 7 0"/><path d="M9 9.5h.01"/><path d="M15 9.5h.01"/>`)
	iconInfo        = strokeIcon(`<circle cx="12" cy="12" r="9"/><path d="M12 11v5"/><path d="M12 8h.01"/>`)
	iconLeave       = strokeIcon(`<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="m16 17 5-5-5-5"/><path d="M21 12H9"/>`)
	iconLink        = strokeIcon(`<path d="M10 13a5 5 0 0 0 7.5.5l3-3a5 5 0 0 0-7-7l-1.7 1.7"/><path d="M14 11a5 5 0 0 0-7.5-.5l-3 3a5 5 0 0 0 7 7l1.7-1.7"/>`)
	iconBell        = strokeIcon(`<path d="M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.7 21a2 2 0 0 1-3.4 0"/>`)
	iconWifi        = strokeIcon(`<path d="M5 12.5a10 10 0 0 1 14 0"/><path d="M8.5 16a5 5 0 0 1 7 0"/><path d="M12 20h.01"/><path d="M2 9a15 15 0 0 1 20 0"/>`)
	iconPause       = fillIcon(`<rect x="6" y="4" width="4" height="16" rx="1.2"/><rect x="14" y="4" width="4" height="16" rx="1.2"/>`)
	iconPlay        = fillIcon(`<path d="M7 4.6v14.8c0 .8.9 1.3 1.6.9l12-7.4c.6-.4.6-1.3 0-1.7l-12-7.4C7.9 3.3 7 3.8 7 4.6z"/>`)
	iconChevron     = strokeIcon(`<path d="m6 9 6 6 6-6"/>`)
	iconBack        = strokeIcon(`<path d="m15 18-6-6 6-6"/>`)
	iconFile        = strokeIcon(`<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/>`)
	iconClock       = strokeIcon(`<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>`)
	iconScreenshot  = strokeIcon(`<path d="M4 8V6a2 2 0 0 1 2-2h2"/><path d="M16 4h2a2 2 0 0 1 2 2v2"/><path d="M20 16v2a2 2 0 0 1-2 2h-2"/><path d="M8 20H6a2 2 0 0 1-2-2v-2"/><circle cx="12" cy="12" r="3"/>`)
	iconShotRect    = strokeIcon(`<rect x="4" y="5" width="16" height="14" rx="1.5"/>`)
	iconShotEllipse = strokeIcon(`<ellipse cx="12" cy="12" rx="8.5" ry="6.5"/>`)
	iconShotArrow   = strokeIcon(`<path d="M5 19 19 5"/><path d="M12 5h7v7"/>`)
	iconShotLine    = strokeIcon(`<path d="M4 20 20 4"/>`)
	iconShotPen     = strokeIcon(`<path d="m4 20 4-1 10-10a2.1 2.1 0 0 0-3-3L5 16z"/><path d="m14 7 3 3"/>`)
	iconShotHL      = strokeIcon(`<path d="M6 15 15 6a2.1 2.1 0 0 1 3 3l-9 9z"/><path d="M4 20h7"/><path d="M13 20h7"/>`)
	iconShotEraser  = strokeIcon(`<path d="m4 16 8-8 6 6-8 8H7z"/><path d="M10 22h10"/>`)
	iconShotText    = strokeIcon(`<path d="M5 6h14"/><path d="M12 6v13"/><path d="M9 19h6"/>`)
	iconShotMosaic  = strokeIcon(`<rect x="4" y="4" width="6" height="6"/><rect x="14" y="4" width="6" height="6"/><rect x="4" y="14" width="6" height="6"/><rect x="14" y="14" width="6" height="6"/>`)
	iconShotNumber  = strokeIcon(`<circle cx="12" cy="12" r="8.5"/><path d="M10 9.5 12 8v8"/><path d="M10 16h4"/>`)
	iconUndo        = strokeIcon(`<path d="M8 7H3v5"/><path d="M3 12a9 9 0 1 0 3-6.7L3 8"/>`)
	iconRedo        = strokeIcon(`<path d="M16 7h5v5"/><path d="M21 12a9 9 0 1 1-3-6.7L21 8"/>`)
	iconPin         = strokeIcon(`<path d="M9 4h6l-1 5 4 4H6l4-4z"/><path d="M12 13v8"/>`)
)
