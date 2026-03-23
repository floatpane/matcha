#ifndef MD4C_H
#define MD4C_H

#ifdef __cplusplus
    extern "C" {
#endif

#if defined MD4C_USE_UTF16

    #ifdef _WIN32
        #include <windows.h>
        typedef WCHAR       MD_CHAR;
    #else
        #error MD4C_USE_UTF16 is only supported on Windows.
    #endif
#else
    typedef char            MD_CHAR;
#endif

typedef unsigned MD_SIZE;
typedef unsigned MD_OFFSET;

typedef enum MD_BLOCKTYPE {

    MD_BLOCK_DOC = 0,

    MD_BLOCK_QUOTE,

    MD_BLOCK_UL,

    MD_BLOCK_OL,

    MD_BLOCK_LI,

    MD_BLOCK_HR,

    MD_BLOCK_H,

    MD_BLOCK_CODE,

    MD_BLOCK_HTML,

    MD_BLOCK_P,

    MD_BLOCK_TABLE,
    MD_BLOCK_THEAD,
    MD_BLOCK_TBODY,
    MD_BLOCK_TR,
    MD_BLOCK_TH,
    MD_BLOCK_TD
} MD_BLOCKTYPE;

typedef enum MD_SPANTYPE {

    MD_SPAN_EM,

    MD_SPAN_STRONG,

    MD_SPAN_A,

    MD_SPAN_IMG,

    MD_SPAN_CODE,

    MD_SPAN_DEL,

    MD_SPAN_LATEXMATH,
    MD_SPAN_LATEXMATH_DISPLAY,

    MD_SPAN_WIKILINK,

    MD_SPAN_U
} MD_SPANTYPE;

typedef enum MD_TEXTTYPE {

    MD_TEXT_NORMAL = 0,

    MD_TEXT_NULLCHAR,

    MD_TEXT_BR,
    MD_TEXT_SOFTBR,

    MD_TEXT_ENTITY,

    MD_TEXT_CODE,

    MD_TEXT_HTML,

    MD_TEXT_LATEXMATH
} MD_TEXTTYPE;

typedef enum MD_ALIGN {
    MD_ALIGN_DEFAULT = 0,
    MD_ALIGN_LEFT,
    MD_ALIGN_CENTER,
    MD_ALIGN_RIGHT
} MD_ALIGN;

typedef struct MD_ATTRIBUTE {
    const MD_CHAR* text;
    MD_SIZE size;
    const MD_TEXTTYPE* substr_types;
    const MD_OFFSET* substr_offsets;
} MD_ATTRIBUTE;

typedef struct MD_BLOCK_UL_DETAIL {
    int is_tight;
    MD_CHAR mark;
} MD_BLOCK_UL_DETAIL;

typedef struct MD_BLOCK_OL_DETAIL {
    unsigned start;
    int is_tight;
    MD_CHAR mark_delimiter;
} MD_BLOCK_OL_DETAIL;

typedef struct MD_BLOCK_LI_DETAIL {
    int is_task;
    MD_CHAR task_mark;
    MD_OFFSET task_mark_offset;
} MD_BLOCK_LI_DETAIL;

typedef struct MD_BLOCK_H_DETAIL {
    unsigned level;
} MD_BLOCK_H_DETAIL;

typedef struct MD_BLOCK_CODE_DETAIL {
    MD_ATTRIBUTE info;
    MD_ATTRIBUTE lang;
    MD_CHAR fence_char;
} MD_BLOCK_CODE_DETAIL;

typedef struct MD_BLOCK_TABLE_DETAIL {
    unsigned col_count;
    unsigned head_row_count;
    unsigned body_row_count;
} MD_BLOCK_TABLE_DETAIL;

typedef struct MD_BLOCK_TD_DETAIL {
    MD_ALIGN align;
} MD_BLOCK_TD_DETAIL;

typedef struct MD_SPAN_A_DETAIL {
    MD_ATTRIBUTE href;
    MD_ATTRIBUTE title;
    int is_autolink;
} MD_SPAN_A_DETAIL;

typedef struct MD_SPAN_IMG_DETAIL {
    MD_ATTRIBUTE src;
    MD_ATTRIBUTE title;
} MD_SPAN_IMG_DETAIL;

typedef struct MD_SPAN_WIKILINK {
    MD_ATTRIBUTE target;
} MD_SPAN_WIKILINK_DETAIL;

#define MD_FLAG_COLLAPSEWHITESPACE          0x0001
#define MD_FLAG_PERMISSIVEATXHEADERS        0x0002
#define MD_FLAG_PERMISSIVEURLAUTOLINKS      0x0004
#define MD_FLAG_PERMISSIVEEMAILAUTOLINKS    0x0008
#define MD_FLAG_NOINDENTEDCODEBLOCKS        0x0010
#define MD_FLAG_NOHTMLBLOCKS                0x0020
#define MD_FLAG_NOHTMLSPANS                 0x0040
#define MD_FLAG_TABLES                      0x0100
#define MD_FLAG_STRIKETHROUGH               0x0200
#define MD_FLAG_PERMISSIVEWWWAUTOLINKS      0x0400
#define MD_FLAG_TASKLISTS                   0x0800
#define MD_FLAG_LATEXMATHSPANS              0x1000
#define MD_FLAG_WIKILINKS                   0x2000
#define MD_FLAG_UNDERLINE                   0x4000
#define MD_FLAG_HARD_SOFT_BREAKS            0x8000

#define MD_FLAG_PERMISSIVEAUTOLINKS         (MD_FLAG_PERMISSIVEEMAILAUTOLINKS | MD_FLAG_PERMISSIVEURLAUTOLINKS | MD_FLAG_PERMISSIVEWWWAUTOLINKS)
#define MD_FLAG_NOHTML                      (MD_FLAG_NOHTMLBLOCKS | MD_FLAG_NOHTMLSPANS)

#define MD_DIALECT_COMMONMARK               0
#define MD_DIALECT_GITHUB                   (MD_FLAG_PERMISSIVEAUTOLINKS | MD_FLAG_TABLES | MD_FLAG_STRIKETHROUGH | MD_FLAG_TASKLISTS)

typedef struct MD_PARSER {

    unsigned abi_version;

    unsigned flags;

    int (*enter_block)(MD_BLOCKTYPE , void* , void* );
    int (*leave_block)(MD_BLOCKTYPE , void* , void* );

    int (*enter_span)(MD_SPANTYPE , void* , void* );
    int (*leave_span)(MD_SPANTYPE , void* , void* );

    int (*text)(MD_TEXTTYPE , const MD_CHAR* , MD_SIZE , void* );

    void (*debug_log)(const char* , void* );

    void (*syntax)(void);
} MD_PARSER;

typedef MD_PARSER MD_RENDERER;

int md_parse(const MD_CHAR* text, MD_SIZE size, const MD_PARSER* parser, void* userdata);

#ifdef __cplusplus
    }
#endif

#endif
