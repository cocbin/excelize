// Copyright 2016 - 2021 The excelize Authors. All rights reserved. Use of
// this source code is governed by a BSD-style license that can be found in
// the LICENSE file.
//
// Package excelize providing a set of functions that allow you to write to
// and read from XLSX / XLSM / XLTM files. Supports reading and writing
// spreadsheet documents generated by Microsoft Excel™ 2007 and later. Supports
// complex components by high compatibility, and provided streaming API for
// generating or reading data from a worksheet with huge amounts of data. This
// library needs Go version 1.15 or later.

package excelize

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"image"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// parseFormatPictureSet provides a function to parse the format settings of
// the picture with default value.
func parseFormatPictureSet(formatSet string) (*formatPicture, error) {
	format := formatPicture{
		FPrintsWithSheet: true,
		FLocksWithSheet:  false,
		NoChangeAspect:   false,
		Autofit:          false,
		OffsetX:          0,
		OffsetY:          0,
		XScale:           1.0,
		YScale:           1.0,
	}
	err := json.Unmarshal(parseFormatSet(formatSet), &format)
	return &format, err
}

// AddPicture provides the method to add picture in a sheet by given picture
// format set (such as offset, scale, aspect ratio setting and print settings)
// and file path. For example:
//
//    package main
//
//    import (
//        _ "image/gif"
//        _ "image/jpeg"
//        _ "image/png"
//
//        "github.com/xuri/excelize/v2"
//    )
//
//    func main() {
//        f := excelize.NewFile()
//        // Insert a picture.
//        if err := f.AddPicture("Sheet1", "A2", "image.jpg", ""); err != nil {
//            fmt.Println(err)
//        }
//        // Insert a picture scaling in the cell with location hyperlink.
//        if err := f.AddPicture("Sheet1", "D2", "image.png", `{"x_scale": 0.5, "y_scale": 0.5, "hyperlink": "#Sheet2!D8", "hyperlink_type": "Location"}`); err != nil {
//            fmt.Println(err)
//        }
//        // Insert a picture offset in the cell with external hyperlink, printing and positioning support.
//        if err := f.AddPicture("Sheet1", "H2", "image.gif", `{"x_offset": 15, "y_offset": 10, "hyperlink": "https://github.com/xuri/excelize", "hyperlink_type": "External", "print_obj": true, "lock_aspect_ratio": false, "locked": false, "positioning": "oneCell"}`); err != nil {
//            fmt.Println(err)
//        }
//        if err := f.SaveAs("Book1.xlsx"); err != nil {
//            fmt.Println(err)
//        }
//    }
//
// LinkType defines two types of hyperlink "External" for web site or
// "Location" for moving to one of cell in this workbook. When the
// "hyperlink_type" is "Location", coordinates need to start with "#".
//
// Positioning defines two types of the position of a picture in an Excel
// spreadsheet, "oneCell" (Move but don't size with cells) or "absolute"
// (Don't move or size with cells). If you don't set this parameter, default
// positioning is move and size with cells.
func (f *File) AddPicture(sheet, cell, picture, format string) error {
	var err error
	// Check picture exists first.
	if _, err = os.Stat(picture); os.IsNotExist(err) {
		return err
	}
	ext, ok := supportImageTypes[path.Ext(picture)]
	if !ok {
		return ErrImgExt
	}
	file, _ := ioutil.ReadFile(picture)
	_, name := filepath.Split(picture)
	return f.AddPictureFromBytes(sheet, cell, format, name, ext, file)
}

// AddPictureFromBytes provides the method to add picture in a sheet by given
// picture format set (such as offset, scale, aspect ratio setting and print
// settings), file base name, extension name and file bytes. For example:
//
//    package main
//
//    import (
//        "fmt"
//        _ "image/jpeg"
//        "io/ioutil"
//
//        "github.com/xuri/excelize/v2"
//    )
//
//    func main() {
//        f := excelize.NewFile()
//
//        file, err := ioutil.ReadFile("image.jpg")
//        if err != nil {
//            fmt.Println(err)
//        }
//        if err := f.AddPictureFromBytes("Sheet1", "A2", "", "Excel Logo", ".jpg", file); err != nil {
//            fmt.Println(err)
//        }
//        if err := f.SaveAs("Book1.xlsx"); err != nil {
//            fmt.Println(err)
//        }
//    }
//
func (f *File) AddPictureFromBytes(sheet, cell, format, name, extension string, file []byte) error {
	var drawingHyperlinkRID int
	var hyperlinkType string
	ext, ok := supportImageTypes[extension]
	if !ok {
		return ErrImgExt
	}
	formatSet, err := parseFormatPictureSet(format)
	if err != nil {
		return err
	}
	img, _, err := image.DecodeConfig(bytes.NewReader(file))
	if err != nil {
		return err
	}
	// Read sheet data.
	ws, err := f.workSheetReader(sheet)
	if err != nil {
		return err
	}
	// Add first picture for given sheet, create xl/drawings/ and xl/drawings/_rels/ folder.
	drawingID := f.countDrawings() + 1
	drawingXML := "xl/drawings/drawing" + strconv.Itoa(drawingID) + ".xml"
	drawingID, drawingXML = f.prepareDrawing(ws, drawingID, sheet, drawingXML)
	drawingRels := "xl/drawings/_rels/drawing" + strconv.Itoa(drawingID) + ".xml.rels"
	mediaStr := ".." + strings.TrimPrefix(f.addMedia(file, ext), "xl")
	drawingRID := f.addRels(drawingRels, SourceRelationshipImage, mediaStr, hyperlinkType)
	// Add picture with hyperlink.
	if formatSet.Hyperlink != "" && formatSet.HyperlinkType != "" {
		if formatSet.HyperlinkType == "External" {
			hyperlinkType = formatSet.HyperlinkType
		}
		drawingHyperlinkRID = f.addRels(drawingRels, SourceRelationshipHyperLink, formatSet.Hyperlink, hyperlinkType)
	}
	err = f.addDrawingPicture(sheet, drawingXML, cell, name, img.Width, img.Height, drawingRID, drawingHyperlinkRID, formatSet)
	if err != nil {
		return err
	}
	f.addContentTypePart(drawingID, "drawings")
	f.addSheetNameSpace(sheet, SourceRelationship)
	return err
}

// deleteSheetRelationships provides a function to delete relationships in
// xl/worksheets/_rels/sheet%d.xml.rels by given worksheet name and
// relationship index.
func (f *File) deleteSheetRelationships(sheet, rID string) {
	name, ok := f.sheetMap[trimSheetName(sheet)]
	if !ok {
		name = strings.ToLower(sheet) + ".xml"
	}
	var rels = "xl/worksheets/_rels/" + strings.TrimPrefix(name, "xl/worksheets/") + ".rels"
	sheetRels := f.relsReader(rels)
	if sheetRels == nil {
		sheetRels = &xlsxRelationships{}
	}
	sheetRels.Lock()
	defer sheetRels.Unlock()
	for k, v := range sheetRels.Relationships {
		if v.ID == rID {
			sheetRels.Relationships = append(sheetRels.Relationships[:k], sheetRels.Relationships[k+1:]...)
		}
	}
	f.Relationships.Store(rels, sheetRels)
}

// addSheetLegacyDrawing provides a function to add legacy drawing element to
// xl/worksheets/sheet%d.xml by given worksheet name and relationship index.
func (f *File) addSheetLegacyDrawing(sheet string, rID int) {
	xlsx, _ := f.workSheetReader(sheet)
	xlsx.LegacyDrawing = &xlsxLegacyDrawing{
		RID: "rId" + strconv.Itoa(rID),
	}
}

// addSheetDrawing provides a function to add drawing element to
// xl/worksheets/sheet%d.xml by given worksheet name and relationship index.
func (f *File) addSheetDrawing(sheet string, rID int) {
	xlsx, _ := f.workSheetReader(sheet)
	xlsx.Drawing = &xlsxDrawing{
		RID: "rId" + strconv.Itoa(rID),
	}
}

// addSheetPicture provides a function to add picture element to
// xl/worksheets/sheet%d.xml by given worksheet name and relationship index.
func (f *File) addSheetPicture(sheet string, rID int) {
	xlsx, _ := f.workSheetReader(sheet)
	xlsx.Picture = &xlsxPicture{
		RID: "rId" + strconv.Itoa(rID),
	}
}

// countDrawings provides a function to get drawing files count storage in the
// folder xl/drawings.
func (f *File) countDrawings() int {
	c1, c2 := 0, 0
	f.Pkg.Range(func(k, v interface{}) bool {
		if strings.Contains(k.(string), "xl/drawings/drawing") {
			c1++
		}
		return true
	})
	f.Drawings.Range(func(rel, value interface{}) bool {
		if strings.Contains(rel.(string), "xl/drawings/drawing") {
			c2++
		}
		return true
	})
	if c1 < c2 {
		return c2
	}
	return c1
}

// addDrawingPicture provides a function to add picture by given sheet,
// drawingXML, cell, file name, width, height relationship index and format
// sets.
func (f *File) addDrawingPicture(sheet, drawingXML, cell, file string, width, height, rID, hyperlinkRID int, formatSet *formatPicture) error {
	col, row, err := CellNameToCoordinates(cell)
	if err != nil {
		return err
	}
	if formatSet.Autofit {
		width, height, col, row, err = f.drawingResize(sheet, cell, float64(width), float64(height), formatSet)
		if err != nil {
			return err
		}
	} else {
		width = int(float64(width) * formatSet.XScale)
		height = int(float64(height) * formatSet.YScale)
	}
	col--
	row--
	colStart, rowStart, colEnd, rowEnd, x2, y2 :=
		f.positionObjectPixels(sheet, col, row, formatSet.OffsetX, formatSet.OffsetY, width, height)
	content, cNvPrID := f.drawingParser(drawingXML)
	twoCellAnchor := xdrCellAnchor{}
	twoCellAnchor.EditAs = formatSet.Positioning
	from := xlsxFrom{}
	from.Col = colStart
	from.ColOff = formatSet.OffsetX * EMU
	from.Row = rowStart
	from.RowOff = formatSet.OffsetY * EMU
	to := xlsxTo{}
	to.Col = colEnd
	to.ColOff = x2 * EMU
	to.Row = rowEnd
	to.RowOff = y2 * EMU
	twoCellAnchor.From = &from
	twoCellAnchor.To = &to
	pic := xlsxPic{}
	pic.NvPicPr.CNvPicPr.PicLocks.NoChangeAspect = formatSet.NoChangeAspect
	pic.NvPicPr.CNvPr.ID = cNvPrID
	pic.NvPicPr.CNvPr.Descr = file
	pic.NvPicPr.CNvPr.Name = "Picture " + strconv.Itoa(cNvPrID)
	if hyperlinkRID != 0 {
		pic.NvPicPr.CNvPr.HlinkClick = &xlsxHlinkClick{
			R:   SourceRelationship.Value,
			RID: "rId" + strconv.Itoa(hyperlinkRID),
		}
	}
	pic.BlipFill.Blip.R = SourceRelationship.Value
	pic.BlipFill.Blip.Embed = "rId" + strconv.Itoa(rID)
	pic.SpPr.PrstGeom.Prst = "rect"

	twoCellAnchor.Pic = &pic
	twoCellAnchor.ClientData = &xdrClientData{
		FLocksWithSheet:  formatSet.FLocksWithSheet,
		FPrintsWithSheet: formatSet.FPrintsWithSheet,
	}
	content.Lock()
	defer content.Unlock()
	content.TwoCellAnchor = append(content.TwoCellAnchor, &twoCellAnchor)
	f.Drawings.Store(drawingXML, content)
	return err
}

// countMedia provides a function to get media files count storage in the
// folder xl/media/image.
func (f *File) countMedia() int {
	count := 0
	f.Pkg.Range(func(k, v interface{}) bool {
		if strings.Contains(k.(string), "xl/media/image") {
			count++
		}
		return true
	})
	return count
}

// addMedia provides a function to add a picture into folder xl/media/image by
// given file and extension name. Duplicate images are only actually stored once
// and drawings that use it will reference the same image.
func (f *File) addMedia(file []byte, ext string) string {
	count := f.countMedia()
	var name string
	f.Pkg.Range(func(k, existing interface{}) bool {
		if !strings.HasPrefix(k.(string), "xl/media/image") {
			return true
		}
		if bytes.Equal(file, existing.([]byte)) {
			name = k.(string)
			return false
		}
		return true
	})
	if name != "" {
		return name
	}
	media := "xl/media/image" + strconv.Itoa(count+1) + ext
	f.Pkg.Store(media, file)
	return media
}

// setContentTypePartImageExtensions provides a function to set the content
// type for relationship parts and the Main Document part.
func (f *File) setContentTypePartImageExtensions() {
	var imageTypes = map[string]bool{"jpeg": false, "png": false, "gif": false, "tiff": false}
	content := f.contentTypesReader()
	content.Lock()
	defer content.Unlock()
	for _, v := range content.Defaults {
		_, ok := imageTypes[v.Extension]
		if ok {
			imageTypes[v.Extension] = true
		}
	}
	for k, v := range imageTypes {
		if !v {
			content.Defaults = append(content.Defaults, xlsxDefault{
				Extension:   k,
				ContentType: "image/" + k,
			})
		}
	}
}

// setContentTypePartVMLExtensions provides a function to set the content type
// for relationship parts and the Main Document part.
func (f *File) setContentTypePartVMLExtensions() {
	vml := false
	content := f.contentTypesReader()
	content.Lock()
	defer content.Unlock()
	for _, v := range content.Defaults {
		if v.Extension == "vml" {
			vml = true
		}
	}
	if !vml {
		content.Defaults = append(content.Defaults, xlsxDefault{
			Extension:   "vml",
			ContentType: ContentTypeVML,
		})
	}
}

// addContentTypePart provides a function to add content type part
// relationships in the file [Content_Types].xml by given index.
func (f *File) addContentTypePart(index int, contentType string) {
	setContentType := map[string]func(){
		"comments": f.setContentTypePartVMLExtensions,
		"drawings": f.setContentTypePartImageExtensions,
	}
	partNames := map[string]string{
		"chart":         "/xl/charts/chart" + strconv.Itoa(index) + ".xml",
		"chartsheet":    "/xl/chartsheets/sheet" + strconv.Itoa(index) + ".xml",
		"comments":      "/xl/comments" + strconv.Itoa(index) + ".xml",
		"drawings":      "/xl/drawings/drawing" + strconv.Itoa(index) + ".xml",
		"table":         "/xl/tables/table" + strconv.Itoa(index) + ".xml",
		"pivotTable":    "/xl/pivotTables/pivotTable" + strconv.Itoa(index) + ".xml",
		"pivotCache":    "/xl/pivotCache/pivotCacheDefinition" + strconv.Itoa(index) + ".xml",
		"sharedStrings": "/xl/sharedStrings.xml",
	}
	contentTypes := map[string]string{
		"chart":         ContentTypeDrawingML,
		"chartsheet":    ContentTypeSpreadSheetMLChartsheet,
		"comments":      ContentTypeSpreadSheetMLComments,
		"drawings":      ContentTypeDrawing,
		"table":         ContentTypeSpreadSheetMLTable,
		"pivotTable":    ContentTypeSpreadSheetMLPivotTable,
		"pivotCache":    ContentTypeSpreadSheetMLPivotCacheDefinition,
		"sharedStrings": ContentTypeSpreadSheetMLSharedStrings,
	}
	s, ok := setContentType[contentType]
	if ok {
		s()
	}
	content := f.contentTypesReader()
	content.Lock()
	defer content.Unlock()
	for _, v := range content.Overrides {
		if v.PartName == partNames[contentType] {
			return
		}
	}
	content.Overrides = append(content.Overrides, xlsxOverride{
		PartName:    partNames[contentType],
		ContentType: contentTypes[contentType],
	})
}

// getSheetRelationshipsTargetByID provides a function to get Target attribute
// value in xl/worksheets/_rels/sheet%d.xml.rels by given worksheet name and
// relationship index.
func (f *File) getSheetRelationshipsTargetByID(sheet, rID string) string {
	name, ok := f.sheetMap[trimSheetName(sheet)]
	if !ok {
		name = strings.ToLower(sheet) + ".xml"
	}
	var rels = "xl/worksheets/_rels/" + strings.TrimPrefix(name, "xl/worksheets/") + ".rels"
	sheetRels := f.relsReader(rels)
	if sheetRels == nil {
		sheetRels = &xlsxRelationships{}
	}
	sheetRels.Lock()
	defer sheetRels.Unlock()
	for _, v := range sheetRels.Relationships {
		if v.ID == rID {
			return v.Target
		}
	}
	return ""
}

// GetPicture provides a function to get picture base name and raw content
// embed in XLSX by given worksheet and cell name. This function returns the
// file name in XLSX and file contents as []byte data types. For example:
//
//    f, err := excelize.OpenFile("Book1.xlsx")
//    if err != nil {
//        fmt.Println(err)
//        return
//    }
//    file, raw, err := f.GetPicture("Sheet1", "A2")
//    if err != nil {
//        fmt.Println(err)
//        return
//    }
//    if err := ioutil.WriteFile(file, raw, 0644); err != nil {
//        fmt.Println(err)
//    }
//
func (f *File) GetPicture(sheet, cell string) (string, []byte, error) {
	col, row, err := CellNameToCoordinates(cell)
	if err != nil {
		return "", nil, err
	}
	col--
	row--
	ws, err := f.workSheetReader(sheet)
	if err != nil {
		return "", nil, err
	}
	if ws.Drawing == nil {
		return "", nil, err
	}
	target := f.getSheetRelationshipsTargetByID(sheet, ws.Drawing.RID)
	drawingXML := strings.Replace(target, "..", "xl", -1)
	if _, ok := f.Pkg.Load(drawingXML); !ok {
		return "", nil, err
	}
	drawingRelationships := strings.Replace(
		strings.Replace(target, "../drawings", "xl/drawings/_rels", -1), ".xml", ".xml.rels", -1)

	return f.getPicture(row, col, drawingXML, drawingRelationships)
}

func (f *File) GetPictures(sheet string) ([]*Picture, error) {
	ws, err := f.workSheetReader(sheet)
	if err != nil {
		return nil, err
	}
	if ws.Drawing == nil {
		return nil, err
	}
	target := f.getSheetRelationshipsTargetByID(sheet, ws.Drawing.RID)
	drawingXML := strings.Replace(target, "..", "xl", -1)
	if _, ok := f.Pkg.Load(drawingXML); !ok {
		return nil, err
	}
	drawingRelationships := strings.Replace(
		strings.Replace(target, "../drawings", "xl/drawings/_rels", -1), ".xml", ".xml.rels", -1)

	return f.getPictures(drawingXML, drawingRelationships)
}

// DeletePicture provides a function to delete charts in spreadsheet by given
// worksheet and cell name. Note that the image file won't be deleted from the
// document currently.
func (f *File) DeletePicture(sheet, cell string) (err error) {
	col, row, err := CellNameToCoordinates(cell)
	if err != nil {
		return
	}
	col--
	row--
	ws, err := f.workSheetReader(sheet)
	if err != nil {
		return
	}
	if ws.Drawing == nil {
		return
	}
	drawingXML := strings.Replace(f.getSheetRelationshipsTargetByID(sheet, ws.Drawing.RID), "..", "xl", -1)
	return f.deleteDrawing(col, row, drawingXML, "Pic")
}

// getPicture provides a function to get picture base name and raw content
// embed in spreadsheet by given coordinates and drawing relationships.
func (f *File) getPicture(row, col int, drawingXML, drawingRelationships string) (ret string, buf []byte, err error) {
	var (
		wsDr            *xlsxWsDr
		ok              bool
		deWsDr          *decodeWsDr
		drawRel         *xlsxRelationship
		deTwoCellAnchor *decodeTwoCellAnchor
	)

	wsDr, _ = f.drawingParser(drawingXML)
	if ret, buf = f.getPictureFromWsDr(row, col, drawingRelationships, wsDr); len(buf) > 0 {
		return
	}
	deWsDr = new(decodeWsDr)
	if err = f.xmlNewDecoder(bytes.NewReader(namespaceStrictToTransitional(f.readXML(drawingXML)))).
		Decode(deWsDr); err != nil && err != io.EOF {
		err = fmt.Errorf("xml decode error: %s", err)
		return
	}
	err = nil
	for _, anchor := range deWsDr.TwoCellAnchor {
		deTwoCellAnchor = new(decodeTwoCellAnchor)
		if err = f.xmlNewDecoder(strings.NewReader("<decodeTwoCellAnchor>" + anchor.Content + "</decodeTwoCellAnchor>")).
			Decode(deTwoCellAnchor); err != nil && err != io.EOF {
			err = fmt.Errorf("xml decode error: %s", err)
			return
		}
		if err = nil; deTwoCellAnchor.From != nil && deTwoCellAnchor.Pic != nil {
			if deTwoCellAnchor.From.Col == col && deTwoCellAnchor.From.Row == row {
				drawRel = f.getDrawingRelationships(drawingRelationships, deTwoCellAnchor.Pic.BlipFill.Blip.Embed)
				if _, ok = supportImageTypes[filepath.Ext(drawRel.Target)]; ok {
					ret = filepath.Base(drawRel.Target)
					if buffer, _ := f.Pkg.Load(strings.Replace(drawRel.Target, "..", "xl", -1)); buffer != nil {
						buf = buffer.([]byte)
					}
					return
				}
			}
		}
	}
	return
}

type PictureCell struct {
	Row int
	Col int
}
type Picture struct {
	Name string
	From PictureCell
	To PictureCell
	Raw [] byte
}

func (f *File) getPictures(drawingXML, drawingRelationships string) (results[] * Picture, err error) {
	var (
		//wsDr            *xlsxWsDr
		ok              bool
		deWsDr          *decodeWsDr
		drawRel         *xlsxRelationship
		deTwoCellAnchor *decodeTwoCellAnchor
	)

	//wsDr, _ = f.drawingParser(drawingXML)
	//if ret, buf = f.getPictureFromWsDr(row, col, drawingRelationships, wsDr); len(buf) > 0 {
	//	return
	//}
	deWsDr = new(decodeWsDr)
	if err = f.xmlNewDecoder(bytes.NewReader(namespaceStrictToTransitional(f.readXML(drawingXML)))).
		Decode(deWsDr); err != nil && err != io.EOF {
		err = fmt.Errorf("xml decode error: %s", err)
		return
	}
	err = nil
	for _, anchor := range deWsDr.TwoCellAnchor {
		deTwoCellAnchor = new(decodeTwoCellAnchor)
		if err = f.xmlNewDecoder(strings.NewReader("<decodeTwoCellAnchor>" + anchor.Content + "</decodeTwoCellAnchor>")).
			Decode(deTwoCellAnchor); err != nil && err != io.EOF {
			err = fmt.Errorf("xml decode error: %s", err)
			return
		}
		if err = nil; deTwoCellAnchor.From != nil && deTwoCellAnchor.Pic != nil {
			// if deTwoCellAnchor.From.Col == col && deTwoCellAnchor.From.Row == row {
				drawRel = f.getDrawingRelationships(drawingRelationships, deTwoCellAnchor.Pic.BlipFill.Blip.Embed)
				if _, ok = supportImageTypes[filepath.Ext(drawRel.Target)]; ok {
					ret := filepath.Base(drawRel.Target)
					if buffer, _ := f.Pkg.Load(strings.Replace(drawRel.Target, "..", "xl", -1)); buffer != nil {
						buf := buffer.([]byte)
						results = append(results, &Picture{
							Name: ret,
							Raw: buf,
							From: PictureCell{
								Row: deTwoCellAnchor.From.Row,
								Col: deTwoCellAnchor.From.Col,
							},
							To: PictureCell {
								Row: deTwoCellAnchor.To.Row,
								Col: deTwoCellAnchor.To.Col,	
							},
						})
					}
				}
			// }
		}
	}
	return
}

// getPictureFromWsDr provides a function to get picture base name and raw
// content in worksheet drawing by given coordinates and drawing
// relationships.
func (f *File) getPictureFromWsDr(row, col int, drawingRelationships string, wsDr *xlsxWsDr) (ret string, buf []byte) {
	var (
		ok      bool
		anchor  *xdrCellAnchor
		drawRel *xlsxRelationship
	)
	wsDr.Lock()
	defer wsDr.Unlock()
	for _, anchor = range wsDr.TwoCellAnchor {
		if anchor.From != nil && anchor.Pic != nil {
			if anchor.From.Col == col && anchor.From.Row == row {
				if drawRel = f.getDrawingRelationships(drawingRelationships,
					anchor.Pic.BlipFill.Blip.Embed); drawRel != nil {
					if _, ok = supportImageTypes[filepath.Ext(drawRel.Target)]; ok {
						ret = filepath.Base(drawRel.Target)
						if buffer, _ := f.Pkg.Load(strings.Replace(drawRel.Target, "..", "xl", -1)); buffer != nil {
							buf = buffer.([]byte)
						}
						return
					}
				}
			}
		}
	}
	return
}

// getDrawingRelationships provides a function to get drawing relationships
// from xl/drawings/_rels/drawing%s.xml.rels by given file name and
// relationship ID.
func (f *File) getDrawingRelationships(rels, rID string) *xlsxRelationship {
	if drawingRels := f.relsReader(rels); drawingRels != nil {
		drawingRels.Lock()
		defer drawingRels.Unlock()
		for _, v := range drawingRels.Relationships {
			if v.ID == rID {
				return &v
			}
		}
	}
	return nil
}

// drawingsWriter provides a function to save xl/drawings/drawing%d.xml after
// serialize structure.
func (f *File) drawingsWriter() {
	f.Drawings.Range(func(path, d interface{}) bool {
		if d != nil {
			v, _ := xml.Marshal(d.(*xlsxWsDr))
			f.saveFileList(path.(string), v)
		}
		return true
	})
}

// drawingResize calculate the height and width after resizing.
func (f *File) drawingResize(sheet string, cell string, width, height float64, formatSet *formatPicture) (w, h, c, r int, err error) {
	var mergeCells []MergeCell
	mergeCells, err = f.GetMergeCells(sheet)
	if err != nil {
		return
	}
	var rng []int
	var inMergeCell bool
	if c, r, err = CellNameToCoordinates(cell); err != nil {
		return
	}
	cellWidth, cellHeight := f.getColWidth(sheet, c), f.getRowHeight(sheet, r)
	for _, mergeCell := range mergeCells {
		if inMergeCell {
			continue
		}
		if inMergeCell, err = f.checkCellInArea(cell, mergeCell[0]); err != nil {
			return
		}
		if inMergeCell {
			rng, _ = areaRangeToCoordinates(mergeCell.GetStartAxis(), mergeCell.GetEndAxis())
			_ = sortCoordinates(rng)
		}
	}
	if inMergeCell {
		cellWidth, cellHeight = 0, 0
		c, r = rng[0], rng[1]
		for col := rng[0]; col <= rng[2]; col++ {
			cellWidth += f.getColWidth(sheet, col)
		}
		for row := rng[1]; row <= rng[3]; row++ {
			cellHeight += f.getRowHeight(sheet, row)
		}
	}
	if float64(cellWidth) < width {
		asp := float64(cellWidth) / width
		width, height = float64(cellWidth), height*asp
	}
	if float64(cellHeight) < height {
		asp := float64(cellHeight) / height
		height, width = float64(cellHeight), width*asp
	}
	width, height = width-float64(formatSet.OffsetX), height-float64(formatSet.OffsetY)
	w, h = int(width*formatSet.XScale), int(height*formatSet.YScale)
	return
}
