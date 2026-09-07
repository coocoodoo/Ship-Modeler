// Generate a loop atlas using the supplied Patina SMIL engine. No native
// Patina window or DLL is needed by Modeler to play these compiled assets.
use patina_core::{color::Rgba, svg::SvgDoc};
use tiny_skia::{Pixmap, PixmapPaint, Transform};

fn main() {
    let root = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("../../internal/ui/patina");
    for name in ["orbit", "beacon"] {
        let source = std::fs::read_to_string(root.join(format!("{name}.svg"))).unwrap();
        let doc = SvgDoc::parse(&source).unwrap();
        assert!(doc.is_animated());
        let mut atlas = Pixmap::new(1536, 960).unwrap();
        let mut first = Vec::new();
        for frame in 0..120 {
            let pix = doc.render_at(192, 64, Rgba::WHITE, None, frame as f64 / 30.0).unwrap();
            if frame == 0 { first = pix.data().to_vec(); }
            if frame == 30 { assert_ne!(first, pix.data(), "SVG did not animate"); }
            atlas.draw_pixmap((frame % 8) * 192, (frame / 8) * 64, pix.as_ref(), &PixmapPaint::default(), Transform::identity(), None);
        }
        atlas.save_png(root.join(format!("{name}-motion.png"))).unwrap();
        println!("Generated {name}: 120 native SMIL frames");
    }
}
