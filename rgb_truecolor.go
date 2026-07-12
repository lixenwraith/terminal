package terminal

// Generic TrueColor palette — pure RGB definitions without game semantics
// Game systems and renderers reference these via aliases in their own parameter files
//
// Naming: standard color names where RGB closely matches (CSS, X11, Pantone-adjacent),
// descriptive compound names otherwise. Ordered dark-to-light within each hue group.

var (
	// --- Achromatic ---
	Black      = RGB{0, 0, 0}
	Charcoal   = RGB{5, 5, 5}
	Obsidian   = RGB{20, 20, 30} // Blue-black
	Gunmetal   = RGB{26, 27, 38} // Blue-tinted near-black
	DarkSlate  = RGB{35, 36, 48} // Blue-gray near-black
	DimGray    = RGB{55, 55, 55}
	DarkGray   = RGB{60, 60, 60}
	IronGray   = RGB{80, 80, 80}
	SlateGray  = RGB{80, 80, 90}  // Cool-tinted
	Taupe      = RGB{100, 95, 85} // Warm gray
	Gray       = RGB{120, 120, 120}
	MidGray    = RGB{128, 128, 128}
	CoolSilver = RGB{140, 145, 155} // Blue-tinted silver
	DimSilver  = RGB{155, 155, 155}
	Silver     = RGB{180, 180, 180}
	LightGray  = RGB{200, 200, 200}
	NearWhite  = RGB{250, 250, 250}
	White      = RGB{255, 255, 255}

	// --- Brown / Earth ---
	Chocolate    = RGB{90, 25, 15}
	SaddleBrown  = RGB{101, 67, 33}
	DarkRust     = RGB{140, 35, 25}
	Sienna       = RGB{140, 60, 0}
	DarkPlum     = RGB{60, 30, 40} // Warm dark purple-brown
	BlueCharcoal = RGB{40, 45, 60} // Cool dark blue-gray

	// --- Red ---
	BlackRed     = RGB{50, 15, 15}
	Oxblood      = RGB{100, 20, 20}
	DarkBurgundy = RGB{100, 25, 20}
	DarkCrimson  = RGB{139, 0, 0}
	Brick        = RGB{180, 40, 40}
	Cinnabar     = RGB{200, 60, 50}
	IndianRed    = RGB{180, 60, 60}
	BurntSienna  = RGB{200, 60, 25}
	Vermilion    = RGB{227, 66, 82}
	Red          = RGB{255, 0, 0}
	BrightRed    = RGB{255, 60, 60}
	Coral        = RGB{255, 80, 80}
	Salmon       = RGB{255, 100, 100}
	LightCoral   = RGB{255, 140, 140}
	LightRose    = RGB{255, 150, 150}
	MistyRose    = RGB{255, 200, 200}

	// --- Orange ---
	DarkAmber   = RGB{60, 40, 0}
	Rust        = RGB{180, 60, 20}
	Amber       = RGB{180, 120, 0}
	Bronze      = RGB{200, 100, 0}
	BurntOrange = RGB{200, 110, 0}
	Terracotta  = RGB{220, 100, 50}
	FlameOrange = RGB{240, 100, 30}
	OrangeRed   = RGB{255, 69, 0}
	RedOrange   = RGB{255, 80, 40}
	Mango       = RGB{255, 120, 50}
	TigerOrange = RGB{255, 140, 0}
	WarmOrange  = RGB{255, 140, 40}
	Apricot     = RGB{255, 160, 60}
	Orange      = RGB{255, 165, 0}

	// --- Yellow ---
	DarkGold    = RGB{200, 150, 0}
	OliveYellow = RGB{200, 180, 60}
	Gold        = RGB{255, 215, 0}
	LemonYellow = RGB{255, 240, 60}
	Yellow      = RGB{255, 255, 0}
	PaleGold    = RGB{255, 200, 100}
	Buttercream = RGB{255, 250, 150}
	PaleLemon   = RGB{255, 255, 100}
	Ivory       = RGB{255, 255, 220}
	Cream       = RGB{255, 255, 200}

	// --- Green ---
	BlackGreen   = RGB{0, 40, 0}
	DarkFern     = RGB{30, 80, 25}
	DeepForest   = RGB{25, 80, 35}
	HunterGreen  = RGB{35, 90, 30}
	DarkGreen    = RGB{15, 130, 15}
	ForestGreen  = RGB{34, 139, 34}
	MediumGreen  = RGB{40, 150, 40}
	FernGreen    = RGB{50, 140, 45}
	LeafGreen    = RGB{60, 160, 60}
	SeaGreen     = RGB{60, 180, 80}
	SageGreen    = RGB{70, 170, 100}
	GrassGreen   = RGB{70, 180, 55}
	EmeraldGreen = RGB{60, 220, 100}
	MintGreen    = RGB{100, 220, 130}
	BrightGreen  = RGB{20, 200, 20}
	YellowGreen  = RGB{100, 220, 80}
	LimeGreen    = RGB{50, 205, 50}
	NeonGreen    = RGB{50, 255, 50}
	BrightLime   = RGB{120, 255, 80}
	Lime         = RGB{0, 255, 0}
	LightGreen   = RGB{144, 238, 144}
	PaleGreen    = RGB{120, 255, 120}
	PastelGreen  = RGB{100, 220, 100}
	PaleMint     = RGB{150, 255, 180}
	Honeydew     = RGB{200, 255, 200}

	// --- Cyan / Teal ---
	Teal          = RGB{0, 139, 139}
	DimCyan       = RGB{0, 160, 160}
	VibrantCyan   = RGB{0, 200, 200}
	DarkTurquoise = RGB{0, 206, 209}
	BrightCyan    = RGB{0, 220, 220}
	Cyan          = RGB{0, 255, 255}
	SkyTeal       = RGB{80, 200, 220}
	PaleCyan      = RGB{200, 255, 255}
	AliceBlue     = RGB{230, 245, 255}
	IceCyan       = RGB{240, 255, 255}

	// --- Blue ---
	DeepNavy     = RGB{15, 25, 50}
	DeepIndigo   = RGB{40, 0, 180}
	NavyBlue     = RGB{30, 60, 120}
	CobaltBlue   = RGB{50, 80, 200}
	SteelBlue    = RGB{60, 100, 180}
	MediumBlue   = RGB{60, 120, 200}
	RoyalBlue    = RGB{65, 105, 225}
	CeruleanBlue = RGB{80, 140, 220}
	Cornflower   = RGB{80, 130, 255}
	DodgerBlue   = RGB{40, 180, 255}
	LightBlue    = RGB{120, 170, 255}
	LightSkyBlue = RGB{135, 206, 250}
	BabyBlue     = RGB{160, 210, 255}
	Blue         = RGB{0, 0, 255}

	// --- Purple / Violet ---
	DeepPurple     = RGB{60, 20, 80}
	DarkViolet     = RGB{120, 40, 180}
	MutedPurple    = RGB{160, 100, 160}
	MediumPurple   = RGB{170, 100, 210}
	ElectricViolet = RGB{180, 130, 255}
	LightOrchid    = RGB{200, 130, 210}
	Orchid         = RGB{200, 120, 220}
	PaleVioletRed  = RGB{219, 112, 147}
	SoftLavender   = RGB{220, 150, 230}
	PaleLavender   = RGB{220, 180, 255}

	// --- Pink / Rose ---
	RoseRed    = RGB{255, 60, 120}
	HotMagenta = RGB{255, 60, 200}
	HotPink    = RGB{255, 140, 200}
	PalePink   = RGB{255, 145, 220}
	LightPink  = RGB{255, 182, 193}
	Pink       = RGB{255, 192, 203}
	Magenta    = RGB{255, 0, 255}
)